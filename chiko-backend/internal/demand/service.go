package demand

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/ws"
)

type Service struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewService(db *pgxpool.Pool, hub *ws.Hub) *Service {
	return &Service{db: db, hub: hub}
}

// isParticipant returns error if callerID is not a member of the chat.
func (s *Service) isParticipant(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND (producer_id=$2 OR client_id=$2))
	`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Msg("demand: isParticipant DB error")
		return errValidation("not a participant of this chat")
	}
	if !exists {
		return errValidation("not a participant of this chat")
	}
	return nil
}

// List returns demand items ordered by urgency (urgent first), then creation time.
func (s *Service) List(ctx context.Context, chatID, callerID uuid.UUID) ([]Item, error) {
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, created_by, name, qty, unit,
		       COALESCE(note,''), urgency, is_included, created_at, updated_at
		FROM demand_items
		WHERE chat_id = $1
		ORDER BY
		    CASE urgency WHEN 'urgent' THEN 0 WHEN 'soon' THEN 1 ELSE 2 END,
		    created_at
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("demand.List: %w", err)
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
			&it.Unit, &it.Note, &it.Urgency, &it.IsIncluded,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Add inserts a new demand item. Any chat participant can add.
func (s *Service) Add(ctx context.Context, in CreateInput, callerID uuid.UUID) (Item, error) {
	if err := s.isParticipant(ctx, in.ChatID, callerID); err != nil {
		return Item{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return Item{}, errValidation("name is required")
	}
	if in.Qty <= 0 {
		return Item{}, errValidation("qty must be > 0")
	}
	if in.Unit == "" {
		in.Unit = "шт"
	}
	if in.Urgency == "" {
		in.Urgency = UrgencyPlanned
	}
	if in.Urgency != UrgencyUrgent && in.Urgency != UrgencySoon && in.Urgency != UrgencyPlanned {
		return Item{}, errValidation("urgency must be urgent, soon, or planned")
	}

	var it Item
	err := s.db.QueryRow(ctx, `
		INSERT INTO demand_items (chat_id, created_by, name, qty, unit, note, urgency)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), urgency, is_included, created_at, updated_at
	`, in.ChatID, callerID, in.Name, in.Qty, in.Unit, nullableStr(in.Note), in.Urgency).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.Urgency, &it.IsIncluded,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, fmt.Errorf("demand.Add: %w", err)
	}

	s.broadcast(it)
	return it, nil
}

// Update edits a demand item. Any participant can edit (producer can mark urgency/included).
func (s *Service) Update(ctx context.Context, itemID, callerID uuid.UUID, in UpdateInput) (Item, error) {
	var chatID uuid.UUID
	if err := s.db.QueryRow(ctx, `SELECT chat_id FROM demand_items WHERE id=$1`, itemID).
		Scan(&chatID); err == pgx.ErrNoRows {
		return Item{}, errValidation("demand item not found")
	} else if err != nil {
		return Item{}, fmt.Errorf("demand.Update load: %w", err)
	}

	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return Item{}, err
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{itemID}

	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return Item{}, errValidation("name cannot be empty")
		}
		args = append(args, *in.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if in.Qty != nil {
		if *in.Qty <= 0 {
			return Item{}, errValidation("qty must be > 0")
		}
		args = append(args, *in.Qty)
		sets = append(sets, fmt.Sprintf("qty = $%d", len(args)))
	}
	if in.Unit != nil {
		args = append(args, *in.Unit)
		sets = append(sets, fmt.Sprintf("unit = $%d", len(args)))
	}
	if in.Note != nil {
		args = append(args, nullableStr(*in.Note))
		sets = append(sets, fmt.Sprintf("note = $%d", len(args)))
	}
	if in.Urgency != nil {
		if *in.Urgency != UrgencyUrgent && *in.Urgency != UrgencySoon && *in.Urgency != UrgencyPlanned {
			return Item{}, errValidation("urgency must be urgent, soon, or planned")
		}
		args = append(args, *in.Urgency)
		sets = append(sets, fmt.Sprintf("urgency = $%d", len(args)))
	}
	if in.IsIncluded != nil {
		args = append(args, *in.IsIncluded)
		sets = append(sets, fmt.Sprintf("is_included = $%d", len(args)))
	}

	var it Item
	q := fmt.Sprintf(`
		UPDATE demand_items SET %s WHERE id = $1
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), urgency, is_included, created_at, updated_at
	`, strings.Join(sets, ", "))
	if err := s.db.QueryRow(ctx, q, args...).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.Urgency, &it.IsIncluded,
		&it.CreatedAt, &it.UpdatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("demand.Update: %w", err)
	}

	s.broadcast(it)
	return it, nil
}

// Remove deletes a demand item. Only the creator can delete.
func (s *Service) Remove(ctx context.Context, itemID, callerID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM demand_items WHERE id=$1 AND created_by=$2
	`, itemID, callerID)
	if err != nil {
		return fmt.Errorf("demand.Remove: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("item not found or you are not the creator")
	}
	return nil
}

// GetSuggestions returns each demand item paired with up to 3 catalog matches
// from the producer's own products — ordered by similarity score descending.
// The PRODUCER reviews the suggestions and explicitly picks which product maps
// to which demand item. No automatic assignment.
func (s *Service) GetSuggestions(ctx context.Context, chatID, producerID uuid.UUID) ([]DemandSuggestion, error) {
	if err := s.isParticipant(ctx, chatID, producerID); err != nil {
		return nil, err
	}

	items, err := s.List(ctx, chatID, producerID)
	if err != nil {
		return nil, err
	}

	suggestions := make([]DemandSuggestion, 0, len(items))
	for _, it := range items {
		if it.IsIncluded {
			continue // already in a draft order — skip
		}

		rows, err := s.db.Query(ctx, `
			SELECT p.id, p.name, p.price, p.unit, COALESCE(p.stock_qty, 0),
			       similarity(p.name, $2) AS score
			FROM   products p
			WHERE  p.producer_id = $1
			  AND  p.is_active   = TRUE
			  AND  similarity(p.name, $2) > 0.15
			ORDER  BY score DESC
			LIMIT  3
		`, producerID, it.Name)
		if err != nil {
			return nil, fmt.Errorf("demand.GetSuggestions query: %w", err)
		}

		var matches []ProductMatch
		for rows.Next() {
			var m ProductMatch
			if err := rows.Scan(&m.ProductID, &m.ProductName, &m.Price, &m.Unit, &m.StockQty, &m.Score); err != nil {
				rows.Close()
				return nil, err
			}
			matches = append(matches, m)
		}
		rows.Close()

		if matches == nil {
			matches = []ProductMatch{}
		}

		suggestions = append(suggestions, DemandSuggestion{
			DemandItem: it,
			Matches:    matches,
		})
	}
	return suggestions, nil
}

// CreateDraftFromMappings creates a draft order using EXPLICIT producer-selected
// product mappings (no auto-assignment). Marks demand items as is_included=true.
// Returns the new draft order ID.
func (s *Service) CreateDraftFromMappings(ctx context.Context, chatID, producerID uuid.UUID, mappings []Mapping) (uuid.UUID, error) {
	if err := s.isParticipant(ctx, chatID, producerID); err != nil {
		return uuid.Nil, err
	}
	if len(mappings) == 0 {
		return uuid.Nil, errValidation("at least one mapping is required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create draft order.
	var orderID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO orders (chat_id, created_by, current_items_jsonb, total)
		VALUES ($1, $2, '[]'::jsonb, 0)
		RETURNING id
	`, chatID, producerID).Scan(&orderID); err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft insert order: %w", err)
	}

	for _, m := range mappings {
		// Verify demand item belongs to this chat and load qty.
		var dQty float64
		err := tx.QueryRow(ctx, `
			SELECT qty FROM demand_items WHERE id=$1 AND chat_id=$2
		`, m.DemandItemID, chatID).Scan(&dQty)
		if err == pgx.ErrNoRows {
			continue // stale mapping — skip silently
		}
		if err != nil {
			return uuid.Nil, err
		}

		// Verify product belongs to producer's catalog.
		var price float64
		err = tx.QueryRow(ctx, `
			SELECT price FROM products
			WHERE id=$1 AND producer_id=$2 AND is_active=TRUE
		`, m.ProductID, producerID).Scan(&price)
		if err == pgx.ErrNoRows {
			continue // product not in catalog — skip
		}
		if err != nil {
			return uuid.Nil, err
		}

		// Insert order_item.
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, qty, price)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (order_id, product_id) DO UPDATE SET qty = EXCLUDED.qty
		`, orderID, m.ProductID, dQty, price); err != nil {
			return uuid.Nil, err
		}

		// Mark demand item as included.
		tx.Exec(ctx, `UPDATE demand_items SET is_included=TRUE WHERE id=$1`, m.DemandItemID)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft commit: %w", err)
	}

	return orderID, nil
}

func (s *Service) broadcast(it Item) {
	payload := map[string]any{
		"id":          it.ID,
		"chat_id":     it.ChatID,
		"name":        it.Name,
		"qty":         it.Qty,
		"unit":        it.Unit,
		"note":        it.Note,
		"urgency":     it.Urgency,
		"is_included": it.IsIncluded,
		"created_by":  it.CreatedBy,
	}
	ev, err := ws.NewEvent(ws.EventDemandUpdated, payload)
	if err != nil {
		return
	}
	encoded, _ := ev.Encode()
	s.hub.Broadcast(ws.BroadcastMsg{ChatID: it.ChatID, Data: encoded})
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
