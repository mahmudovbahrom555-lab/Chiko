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

// List returns all demand items for a chat ordered by creation time.
func (s *Service) List(ctx context.Context, chatID, callerID uuid.UUID) ([]Item, error) {
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, created_by, name, qty, unit,
		       COALESCE(note,''), is_filled, created_at, updated_at
		FROM demand_items
		WHERE chat_id = $1
		ORDER BY created_at
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("demand.List: %w", err)
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
			&it.Unit, &it.Note, &it.IsFilled, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Add inserts a new demand item. Any chat participant can add items.
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

	var it Item
	err := s.db.QueryRow(ctx, `
		INSERT INTO demand_items (chat_id, created_by, name, qty, unit, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), is_filled, created_at, updated_at
	`, in.ChatID, callerID, in.Name, in.Qty, in.Unit, nullableString(in.Note)).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.IsFilled, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, fmt.Errorf("demand.Add: %w", err)
	}

	s.broadcast(ws.EventDemandUpdated, it)
	return it, nil
}

// Update edits an existing demand item.
// Only the creator or the producer side can edit (both are chat participants).
func (s *Service) Update(ctx context.Context, itemID, callerID uuid.UUID, in UpdateInput) (Item, error) {
	// Load item to get chat_id for participant check.
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

	// Build partial SET clause.
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
		args = append(args, nullableString(*in.Note))
		sets = append(sets, fmt.Sprintf("note = $%d", len(args)))
	}
	if in.IsFilled != nil {
		args = append(args, *in.IsFilled)
		sets = append(sets, fmt.Sprintf("is_filled = $%d", len(args)))
	}

	var it Item
	q := fmt.Sprintf(`
		UPDATE demand_items SET %s WHERE id = $1
		RETURNING id, chat_id, created_by, name, qty, unit,
		          COALESCE(note,''), is_filled, created_at, updated_at
	`, strings.Join(sets, ", "))
	err := s.db.QueryRow(ctx, q, args...).Scan(
		&it.ID, &it.ChatID, &it.CreatedBy, &it.Name, &it.Qty,
		&it.Unit, &it.Note, &it.IsFilled, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, fmt.Errorf("demand.Update: %w", err)
	}

	s.broadcast(ws.EventDemandUpdated, it)
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
	s.broadcastRemove(itemID)
	return nil
}

// CreateDraftFromDemand creates a draft order pre-filled with demand items
// that exist in the producer's catalog (matched by name fuzzy search).
// Items without a catalog match are skipped — producer adds them manually.
// Returns the new order ID so the handler can redirect to the order editor.
func (s *Service) CreateDraftFromDemand(ctx context.Context, chatID, producerID uuid.UUID, itemIDs []uuid.UUID) (uuid.UUID, error) {
	if err := s.isParticipant(ctx, chatID, producerID); err != nil {
		return uuid.Nil, err
	}

	// Create draft order.
	var orderID uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO orders (chat_id, created_by, current_items_jsonb, total)
		VALUES ($1, $2, '[]'::jsonb, 0)
		RETURNING id
	`, chatID, producerID).Scan(&orderID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("demand.CreateDraft insert order: %w", err)
	}

	// For each selected demand item, try to match against the producer's catalog
	// by name similarity and insert as an order_item.
	for _, demandID := range itemIDs {
		var dName string
		var dQty float64
		if err := s.db.QueryRow(ctx, `
			SELECT name, qty FROM demand_items WHERE id=$1 AND chat_id=$2
		`, demandID, chatID).Scan(&dName, &dQty); err != nil {
			continue // skip missing items
		}

		// Find best matching product in producer's catalog.
		var productID uuid.UUID
		var price float64
		err := s.db.QueryRow(ctx, `
			SELECT p.id, p.price
			FROM products p
			WHERE p.producer_id = $1
			  AND p.is_active = TRUE
			  AND similarity(p.name, $2) > 0.2
			ORDER BY similarity(p.name, $2) DESC
			LIMIT 1
		`, producerID, dName).Scan(&productID, &price)
		if err != nil {
			continue // no match found — producer adds manually
		}

		// Insert as order_item.
		s.db.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, qty, price)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (order_id, product_id) DO UPDATE SET qty = EXCLUDED.qty
		`, orderID, productID, dQty, price)
	}

	return orderID, nil
}

func (s *Service) broadcast(eventType ws.EventType, it Item) {
	payload := map[string]any{
		"id":         it.ID,
		"chat_id":    it.ChatID,
		"name":       it.Name,
		"qty":        it.Qty,
		"unit":       it.Unit,
		"note":       it.Note,
		"is_filled":  it.IsFilled,
		"created_by": it.CreatedBy,
	}
	ev, err := ws.NewEvent(eventType, payload)
	if err != nil {
		return
	}
	encoded, _ := ev.Encode()
	s.hub.Broadcast(ws.BroadcastMsg{ChatID: it.ChatID, Data: encoded})
}

func (s *Service) broadcastRemove(itemID uuid.UUID) {
	// We no longer have the chatID after delete, so we can't broadcast.
	// In practice the client refreshes the list on 204 response.
	_ = itemID
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
