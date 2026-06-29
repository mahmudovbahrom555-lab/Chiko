package buylist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const guestTTL = 7 * 24 * time.Hour // ТЗ раздел 4.5: 7 дней

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// Create создаёт Buy List без регистрации (anon).
// Ищет producer по номеру телефона в auth.users.
// Если поставщик не найден — заказ создаётся в режиме ожидания
// (producer_id привяжется при регистрации этого номера, ТЗ 5.5).
func (s *Service) Create(ctx context.Context, in CreateInput) (*BuyList, error) {
	if strings.TrimSpace(in.ProducerPhone) == "" {
		return nil, errValidation("producer_phone is required")
	}
	if len(in.Lines) == 0 {
		return nil, errValidation("at least one line is required")
	}

	// Ищем producer по номеру телефона (ТЗ 13.7: через auth.users).
	var chatID *uuid.UUID
	var producerID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT c.id, c.producer_id
		FROM   chats c
		JOIN   auth.users u ON u.id = c.producer_id
		WHERE  u.phone = $1
		LIMIT  1
	`, in.ProducerPhone).Scan(&chatID, &producerID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("buylist.Create lookup producer: %w", err)
	}

	guestToken := uuid.New()
	expiresAt := time.Now().Add(guestTTL)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("buylist.Create begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// INSERT order со status='guest_buy_list'.
	var orderID uuid.UUID
	var chatIDVal uuid.UUID
	if chatID != nil {
		chatIDVal = *chatID
		err = tx.QueryRow(ctx, `
			INSERT INTO orders
			    (chat_id, status, created_by, current_items_jsonb, total,
			     guest_token, created_by_guest, guest_phone, expires_at)
			VALUES ($1, 'guest_buy_list', $2, '[]'::jsonb, 0,
			        $3, TRUE, $4, $5)
			RETURNING id
		`, chatIDVal, producerID, guestToken, nullableStr(in.GuestPhone), expiresAt).Scan(&orderID)
	} else {
		// Поставщик не найден — создаём без chat_id (временное состояние).
		// При регистрации поставщика Go-код привяжет chat (ТЗ 5.5).
		// chat_id обязателен FK — пока сохраняем как pending через guest_phone.
		// Упрощение MVP: возвращаем ошибку если поставщик не зарегистрирован.
		return nil, errValidation("producer not found in Chiko — ask them to register first")
	}
	if err != nil {
		return nil, fmt.Errorf("buylist.Create insert order: %w", err)
	}

	// INSERT строки с raw_text, product_id=NULL.
	lines := make([]Line, 0, len(in.Lines))
	for _, l := range in.Lines {
		text := strings.TrimSpace(l.Text)
		if text == "" || l.Qty <= 0 {
			continue
		}
		var lineID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO order_items (order_id, raw_text, qty, price, added_by)
			VALUES ($1, $2, $3, 0, '00000000-0000-0000-0000-000000000000'::uuid)
			RETURNING id
		`, orderID, text, l.Qty).Scan(&lineID); err != nil {
			return nil, fmt.Errorf("buylist.Create insert line: %w", err)
		}
		lines = append(lines, Line{
			ID:      lineID,
			RawText: text,
			Qty:     l.Qty,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("buylist.Create commit: %w", err)
	}

	return &BuyList{
		ID:            orderID,
		GuestToken:    guestToken,
		ProducerPhone: in.ProducerPhone,
		GuestPhone:    in.GuestPhone,
		Status:        "guest_buy_list",
		Lines:         lines,
		ExpiresAt:     expiresAt,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// GetByToken возвращает Buy List по guest_token (публичный, без auth).
func (s *Service) GetByToken(ctx context.Context, token uuid.UUID) (*BuyList, error) {
	var bl BuyList
	var chatID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT id, chat_id, guest_token, COALESCE(guest_phone,''), status, expires_at, created_at
		FROM   orders
		WHERE  guest_token = $1 AND expires_at > NOW()
	`, token).Scan(
		&bl.ID, &chatID, &bl.GuestToken, &bl.GuestPhone,
		&bl.Status, &bl.ExpiresAt, &bl.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errValidation("buy list not found or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("buylist.GetByToken: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT oi.id, oi.raw_text, oi.product_id, COALESCE(p.name,''), oi.qty, oi.price
		FROM   order_items oi
		LEFT JOIN products p ON p.id = oi.product_id
		WHERE  oi.order_id = $1
		ORDER  BY oi.created_at
	`, bl.ID)
	if err != nil {
		return nil, fmt.Errorf("buylist.GetByToken lines: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var l Line
		var pid *uuid.UUID
		var pname string
		if err := rows.Scan(&l.ID, &l.RawText, &pid, &pname, &l.Qty, &l.Price); err != nil {
			return nil, err
		}
		l.ProductID = pid
		l.ProductName = pname
		bl.Lines = append(bl.Lines, l)
	}
	if bl.Lines == nil {
		bl.Lines = []Line{}
	}
	return &bl, rows.Err()
}

// GetSuggestions возвращает несопоставленные строки с вариантами из каталога.
// Вызывается производителем (auth required) перед MapLines.
func (s *Service) GetSuggestions(ctx context.Context, orderID, producerID uuid.UUID) ([]LineSuggestion, error) {
	// Проверяем что производитель — участник чата этого заказа.
	var chatID uuid.UUID
	if err := s.db.QueryRow(ctx, `
		SELECT chat_id FROM orders
		WHERE id=$1 AND status='guest_buy_list'
	`, orderID).Scan(&chatID); err == pgx.ErrNoRows {
		return nil, errValidation("buy list not found")
	} else if err != nil {
		return nil, err
	}

	var isProducer bool
	s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND producer_id=$2)`,
		chatID, producerID).Scan(&isProducer)
	if !isProducer {
		return nil, errValidation("not the producer of this chat")
	}

	// Загружаем несопоставленные строки.
	rows, err := s.db.Query(ctx, `
		SELECT id, raw_text, qty FROM order_items
		WHERE order_id=$1 AND product_id IS NULL
		ORDER BY created_at
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var unmapped []Line
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.ID, &l.RawText, &l.Qty); err != nil {
			return nil, err
		}
		unmapped = append(unmapped, l)
	}
	rows.Close()

	// Для каждой строки ищем варианты из каталога.
	suggestions := make([]LineSuggestion, 0, len(unmapped))
	for _, l := range unmapped {
		norm := strings.ToLower(strings.TrimSpace(l.RawText))

		// Проверяем buy_list_mappings (кэш прошлых сопоставлений).
		var cachedProductID uuid.UUID
		_ = s.db.QueryRow(ctx, `
			SELECT product_id FROM buy_list_mappings
			WHERE chat_id=$1 AND name_normalized=$2
		`, chatID, norm).Scan(&cachedProductID)

		// Fuzzy match по каталогу производителя.
		mRows, err := s.db.Query(ctx, `
			SELECT p.id, p.name, p.price, p.unit, COALESCE(p.stock_qty,0),
			       similarity(p.name, $3) AS score
			FROM   products p
			WHERE  p.producer_id=$1
			  AND  p.is_active=TRUE
			  AND  similarity(p.name, $3) > 0.15
			ORDER  BY
			    CASE WHEN p.id=$2
			         AND $2 != '00000000-0000-0000-0000-000000000000'::uuid
			         THEN 0 ELSE 1 END,
			    score DESC
			LIMIT  3
		`, producerID, cachedProductID, l.RawText)
		if err != nil {
			return nil, fmt.Errorf("buylist.GetSuggestions match: %w", err)
		}

		var matches []SuggestedMatch
		for mRows.Next() {
			var m SuggestedMatch
			if err := mRows.Scan(&m.ProductID, &m.ProductName, &m.Price,
				&m.Unit, &m.StockQty, &m.Score); err != nil {
				mRows.Close()
				return nil, err
			}
			m.IsCached = m.ProductID == cachedProductID && cachedProductID != uuid.Nil
			matches = append(matches, m)
		}
		mRows.Close()

		if matches == nil {
			matches = []SuggestedMatch{}
		}
		suggestions = append(suggestions, LineSuggestion{Line: l, Matches: matches})
	}
	return suggestions, nil
}

// MapLines производитель сопоставляет строки с товарами из каталога.
// Сохраняет выбор в buy_list_mappings для будущего автосопоставления.
func (s *Service) MapLines(ctx context.Context, orderID, producerID uuid.UUID, mappings []MapInput) (*BuyList, error) {
	var chatID uuid.UUID
	if err := s.db.QueryRow(ctx, `
		SELECT chat_id FROM orders
		WHERE id=$1 AND status='guest_buy_list'
	`, orderID).Scan(&chatID); err == pgx.ErrNoRows {
		return nil, errValidation("buy list not found")
	} else if err != nil {
		return nil, err
	}

	var isProducer bool
	s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND producer_id=$2)`,
		chatID, producerID).Scan(&isProducer)
	if !isProducer {
		return nil, errValidation("not the producer of this chat")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, m := range mappings {
		if m.ProductID == uuid.Nil {
			// "Нет товара" — оставляем raw_text, обнуляем product_id (уже NULL).
			continue
		}

		// Загружаем raw_text и qty строки для кэша и price.
		var rawText string
		var qty float64
		if err := tx.QueryRow(ctx, `
			SELECT raw_text, qty FROM order_items
			WHERE id=$1 AND order_id=$2
		`, m.LineID, orderID).Scan(&rawText, &qty); err == pgx.ErrNoRows {
			continue
		} else if err != nil {
			return nil, err
		}

		// Берём текущую цену из каталога производителя.
		var price float64
		if err := tx.QueryRow(ctx, `
			SELECT price FROM products WHERE id=$1 AND producer_id=$2 AND is_active=TRUE
		`, m.ProductID, producerID).Scan(&price); err == pgx.ErrNoRows {
			continue // товара нет в каталоге — пропускаем
		} else if err != nil {
			return nil, err
		}

		// Обновляем строку: заполняем product_id и price.
		tx.Exec(ctx, `
			UPDATE order_items SET product_id=$1, price=$2 WHERE id=$3
		`, m.ProductID, price, m.LineID)

		// Сохраняем сопоставление в кэш (UPSERT).
		norm := strings.ToLower(strings.TrimSpace(rawText))
		tx.Exec(ctx, `
			INSERT INTO buy_list_mappings (chat_id, name_normalized, product_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (chat_id, name_normalized) DO UPDATE
			    SET product_id=EXCLUDED.product_id, updated_at=NOW()
		`, chatID, norm, m.ProductID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("buylist.MapLines commit: %w", err)
	}

	return s.GetByToken(ctx, uuid.Nil) // вернём обновлённый список через GetByOrderID
}

// GetByOrderID — внутренний метод, используется после MapLines.
func (s *Service) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*BuyList, error) {
	var token uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT guest_token FROM orders WHERE id=$1`, orderID).Scan(&token)
	if err != nil {
		return nil, err
	}
	return s.GetByToken(ctx, token)
}

// ConvertToDraft переводит guest_buy_list → draft при регистрации магазина.
// Вызывается из users/handler.go при bootstrap (ТЗ 13.7).
func (s *Service) ConvertToDraft(ctx context.Context, guestToken uuid.UUID, clientID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE orders
		SET    client_id   = $1,
		       status      = 'draft',
		       guest_token = NULL
		WHERE  guest_token = $2
		  AND  status = 'guest_buy_list'
		  AND  expires_at > NOW()
	`, clientID, guestToken)
	if err != nil {
		return fmt.Errorf("buylist.ConvertToDraft: %w", err)
	}
	if tag.RowsAffected() == 0 {
		log.Debug().Str("token", guestToken.String()).Msg("buylist: no guest order to convert")
	}
	return nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
