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

// List returns ALL demand items for a chat — visible at every status.
// Sorted: urgent first, then by creation time.
// Both sides always see the list; Flutter shows status badge per item.
func (s *Service) List(ctx context.Context, chatID, callerID uuid.UUID) ([]Item, error) {
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, created_by, name, qty, unit,
		       COALESCE(note,''), urgency, status, created_at, updated_at
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
			&it.Unit, &it.Note, &it.Urgency, &it.Status,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Add inserts a demand item with status=open.
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
		          COALESCE(note,''), urgency, status, created_at, updated_at
	`, in.ChatID, callerID, in.Name, in.Qty, in.Unit, nullableStr(in.Note), in.Urgency).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.Urgency, &it.Status,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, fmt.Errorf("demand.Add: %w", err)
	}

	s.broadcast(it)
	s.logEvent(ctx, callerID, "demand_item_added", it.ID)
	return it, nil
}

// Update edits name/qty/unit/note/urgency. Status is managed by the system, not the user.
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

	var it Item
	q := fmt.Sprintf(`
		UPDATE demand_items SET %s WHERE id = $1
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), urgency, status, created_at, updated_at
	`, strings.Join(sets, ", "))
	if err := s.db.QueryRow(ctx, q, args...).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.Urgency, &it.Status,
		&it.CreatedAt, &it.UpdatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("demand.Update: %w", err)
	}

	s.broadcast(it)
	return it, nil
}

// Remove deletes a demand item. Only the creator can delete.
func (s *Service) Remove(ctx context.Context, itemID, callerID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM demand_items WHERE id=$1 AND created_by=$2`, itemID, callerID)
	if err != nil {
		return fmt.Errorf("demand.Remove: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("item not found or you are not the creator")
	}
	return nil
}

// Cancel marks a demand item as cancelled.
// Any chat participant can cancel (магазин передумал, товар куплен у другого).
// The item stays visible with "Отменено" badge — not deleted.
func (s *Service) Cancel(ctx context.Context, itemID, callerID uuid.UUID) (Item, error) {
	var chatID uuid.UUID
	var currentStatus Status
	err := s.db.QueryRow(ctx, `SELECT chat_id, status FROM demand_items WHERE id=$1`, itemID).
		Scan(&chatID, &currentStatus)
	if err == pgx.ErrNoRows {
		return Item{}, errValidation("demand item not found")
	}
	if err != nil {
		return Item{}, fmt.Errorf("demand.Cancel load: %w", err)
	}
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return Item{}, err
	}
	if currentStatus == StatusCancelled {
		return Item{}, errValidation("already cancelled")
	}
	if currentStatus == StatusOrdered {
		return Item{}, errValidation("cannot cancel an already ordered item")
	}

	var it Item
	if err := s.db.QueryRow(ctx, `
		UPDATE demand_items SET status='cancelled', updated_at=NOW() WHERE id=$1
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), urgency, status, created_at, updated_at
	`, itemID).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.Urgency, &it.Status,
		&it.CreatedAt, &it.UpdatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("demand.Cancel: %w", err)
	}

	s.broadcast(it)
	s.logEvent(ctx, callerID, "demand_cancelled", it.ID)
	return it, nil
}

// GetSuggestions returns open demand items paired with catalog matches.
// Preferred products (from demand_preferences) are surfaced first with IsPreferred=true.
// Items with status != open are excluded — they're already in progress.
func (s *Service) GetSuggestions(ctx context.Context, chatID, producerID uuid.UUID) ([]DemandSuggestion, error) {
	if err := s.isParticipant(ctx, chatID, producerID); err != nil {
		return nil, err
	}

	// Load only open items.
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, created_by, name, qty, unit,
		       COALESCE(note,''), urgency, status, created_at, updated_at
		FROM demand_items
		WHERE chat_id=$1 AND status='open'
		ORDER BY CASE urgency WHEN 'urgent' THEN 0 WHEN 'soon' THEN 1 ELSE 2 END, created_at
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("demand.GetSuggestions list: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
			&it.Unit, &it.Note, &it.Urgency, &it.Status,
			&it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	suggestions := make([]DemandSuggestion, 0, len(items))
	for _, it := range items {
		// Check for a preferred product remembered from demand_preferences.
		var preferredID uuid.UUID
		_ = s.db.QueryRow(ctx, `
			SELECT product_id FROM demand_preferences
			WHERE chat_id=$1 AND name_normalized=LOWER(TRIM($2))
		`, chatID, it.Name).Scan(&preferredID)

		// Fuzzy match from catalog — up to 3 candidates.
		mRows, err := s.db.Query(ctx, `
			SELECT p.id, p.name, p.price, p.unit, COALESCE(p.stock_qty,0),
			       similarity(p.name, $3) AS score
			FROM   products p
			WHERE  p.producer_id=$1
			  AND  p.is_active=TRUE
			  AND  similarity(p.name, $3) > 0.15
			ORDER  BY
			    -- preferred product always first regardless of score
			    CASE WHEN p.id=$2 THEN 0 ELSE 1 END,
			    score DESC
			LIMIT  3
		`, producerID, preferredID, it.Name)
		if err != nil {
			return nil, fmt.Errorf("demand.GetSuggestions match: %w", err)
		}

		var matches []ProductMatch
		for mRows.Next() {
			var m ProductMatch
			if err := mRows.Scan(&m.ProductID, &m.ProductName, &m.Price,
				&m.Unit, &m.StockQty, &m.Score); err != nil {
				mRows.Close()
				return nil, err
			}
			m.IsPreferred = m.ProductID == preferredID
			matches = append(matches, m)
		}
		mRows.Close()

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

// CreateDraftFromMappings creates a draft order from producer's explicit choices.
// Atomically: INSERT order + order_items + mark demand_items as proposed
//             + save demand_preferences for future suggestions.
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

	var orderID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO orders (chat_id, created_by, current_items_jsonb, total)
		VALUES ($1, $2, '[]'::jsonb, 0) RETURNING id
	`, chatID, producerID).Scan(&orderID); err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft order: %w", err)
	}

	for _, m := range mappings {
		// Load demand item qty (validates it belongs to this chat).
		var dName string
		var dQty float64
		err := tx.QueryRow(ctx, `
			SELECT name, qty FROM demand_items WHERE id=$1 AND chat_id=$2 AND status='open'
		`, m.DemandItemID, chatID).Scan(&dName, &dQty)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return uuid.Nil, err
		}

		// Validate product belongs to producer's active catalog.
		var price float64
		err = tx.QueryRow(ctx, `
			SELECT price FROM products WHERE id=$1 AND producer_id=$2 AND is_active=TRUE
		`, m.ProductID, producerID).Scan(&price)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return uuid.Nil, err
		}

		// Insert order_item.
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, qty, price)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (order_id, product_id) DO UPDATE SET qty=EXCLUDED.qty
		`, orderID, m.ProductID, dQty, price); err != nil {
			return uuid.Nil, err
		}

		// Mark demand item as proposed.
		tx.Exec(ctx, `UPDATE demand_items SET status='proposed' WHERE id=$1`, m.DemandItemID)

		// Save producer's choice to demand_preferences (Smart Memory).
		tx.Exec(ctx, `
			INSERT INTO demand_preferences (chat_id, name_normalized, product_id)
			VALUES ($1, LOWER(TRIM($2)), $3)
			ON CONFLICT (chat_id, name_normalized) DO UPDATE
			    SET product_id=EXCLUDED.product_id, updated_at=NOW()
		`, chatID, dName, m.ProductID)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft commit: %w", err)
	}

	// Log conversion event: demand → proposed (for Demand→Draft metric).
	for _, m := range mappings {
		s.logEvent(ctx, producerID, "demand_item_proposed", m.DemandItemID)
	}

	return orderID, nil
}

// MarkOrdered transitions proposed→ordered for all proposed demand items in a chat.
// Called by order.Handler after a successful Confirm.
// Also logs demand_item_ordered events for the conversion funnel metric.
func (s *Service) MarkOrdered(ctx context.Context, chatID uuid.UUID) {
	rows, err := s.db.Query(ctx, `
		UPDATE demand_items SET status='ordered', updated_at=NOW()
		WHERE chat_id=$1 AND status='proposed'
		RETURNING id, created_by
	`, chatID)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Msg("demand: MarkOrdered failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var itemID, createdBy uuid.UUID
		if err := rows.Scan(&itemID, &createdBy); err != nil {
			continue
		}
		s.logEvent(ctx, createdBy, "demand_item_ordered", itemID)
	}
}

// logEvent writes a demand lifecycle event to the events table.
// Used for the Demand→Draft→Order conversion funnel metric.
// Fail-silently — analytics must not break the main flow.
func (s *Service) logEvent(ctx context.Context, userID uuid.UUID, eventType string, entityID uuid.UUID) {
	s.db.Exec(ctx, `
		INSERT INTO events (user_id, type, entity_id)
		VALUES ($1, $2, $3)
	`, userID, eventType, entityID)
}

func (s *Service) broadcast(it Item) {
	payload := map[string]any{
		"id":         it.ID,
		"chat_id":    it.ChatID,
		"name":       it.Name,
		"qty":        it.Qty,
		"unit":       it.Unit,
		"note":       it.Note,
		"urgency":    it.Urgency,
		"status":     it.Status,
		"created_by": it.CreatedBy,
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
