package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chiko/backend/internal/ws"
)

// Service handles all collaborative order logic.
// It talks directly to the DB and broadcasts WS events via the Hub.
type Service struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewService(db *pgxpool.Pool, hub *ws.Hub) *Service {
	return &Service{db: db, hub: hub}
}

// ──────────────────────────── CREATE ─────────────────────────────────────────

// CreateDraft creates a new draft order in the given chat.
// Both producer and client may call this (ТЗ раздел 1.2: роль инициатора не фиксирована).
func (s *Service) CreateDraft(ctx context.Context, chatID, callerID uuid.UUID) (Order, error) {
	var o Order
	err := s.db.QueryRow(ctx, `
		INSERT INTO orders (chat_id, created_by, current_items_jsonb, total)
		VALUES ($1, $2, '[]'::jsonb, 0)
		RETURNING id, chat_id, status, created_by, confirmed_by, confirmed_at,
		          current_items_jsonb, total, created_at, updated_at
	`, chatID, callerID).Scan(
		&o.ID, &o.ChatID, &o.Status, &o.CreatedBy, &o.ConfirmedBy, &o.ConfirmedAt,
		&o.CurrentItemsJSON, &o.Total, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return Order{}, fmt.Errorf("order.CreateDraft: %w", err)
	}
	return o, nil
}

// GetOrder returns a single order.
func (s *Service) GetOrder(ctx context.Context, orderID uuid.UUID) (Order, error) {
	var o Order
	err := s.db.QueryRow(ctx, `
		SELECT id, chat_id, status, created_by, confirmed_by, confirmed_at,
		       current_items_jsonb, total, created_at, updated_at
		FROM   orders WHERE id = $1
	`, orderID).Scan(
		&o.ID, &o.ChatID, &o.Status, &o.CreatedBy, &o.ConfirmedBy, &o.ConfirmedAt,
		&o.CurrentItemsJSON, &o.Total, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return Order{}, errValidation("order not found")
	}
	if err != nil {
		return Order{}, fmt.Errorf("order.GetOrder: %w", err)
	}
	return o, nil
}

// ListByChat returns all orders for a chat in reverse chronological order.
func (s *Service) ListByChat(ctx context.Context, chatID uuid.UUID) ([]Order, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, status, created_by, confirmed_by, confirmed_at,
		       current_items_jsonb, total, created_at, updated_at
		FROM   orders
		WHERE  chat_id = $1
		ORDER  BY created_at DESC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("order.ListByChat: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(
			&o.ID, &o.ChatID, &o.Status, &o.CreatedBy, &o.ConfirmedBy, &o.ConfirmedAt,
			&o.CurrentItemsJSON, &o.Total, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// ──────────────────────────── ITEMS ──────────────────────────────────────────

// UpsertItem adds or updates a line in a draft order.
// Soft-lock is acquired; if there's a conflict the losing client gets a WS event.
// After the write, current_items_jsonb snapshot is rebuilt and item_updated is broadcast.
func (s *Service) UpsertItem(ctx context.Context, orderID, callerID uuid.UUID, in UpdateItemInput) (OrderItem, error) {
	if in.Qty <= 0 {
		return OrderItem{}, errValidation("qty must be > 0")
	}

	// 1. Verify order is draft and load its chat_id (needed for broadcast).
	var chatID uuid.UUID
	var status string
	err := s.db.QueryRow(ctx, `
		SELECT chat_id, status FROM orders WHERE id = $1
	`, orderID).Scan(&chatID, &status)
	if err == pgx.ErrNoRows {
		return OrderItem{}, errValidation("order not found")
	}
	if err != nil {
		return OrderItem{}, fmt.Errorf("order.UpsertItem verify: %w", err)
	}
	if status != StatusDraft {
		return OrderItem{}, errValidation("order is not in draft status")
	}

	// 2. Get current product price (ТЗ раздел 4.6: цена из ТЕКУЩЕГО каталога).
	var productPrice float64
	err = s.db.QueryRow(ctx, `
		SELECT price FROM products WHERE id = $1 AND is_active = TRUE
	`, in.ProductID).Scan(&productPrice)
	if err == pgx.ErrNoRows {
		return OrderItem{}, errValidation("product not found or inactive")
	}
	if err != nil {
		return OrderItem{}, fmt.Errorf("order.UpsertItem price: %w", err)
	}

	// 3. Upsert the item row, acquiring the soft lock.
	var item OrderItem
	err = s.db.QueryRow(ctx, `
		INSERT INTO order_items (order_id, product_id, qty, price, added_by,
		                         locked_by, locked_until)
		VALUES ($1, $2, $3, $4, $5, $5, NOW() + INTERVAL '3 seconds')
		ON CONFLICT (order_id, product_id) DO UPDATE
		  SET qty          = EXCLUDED.qty,
		      price        = EXCLUDED.price,
		      locked_by    = EXCLUDED.locked_by,
		      locked_until = EXCLUDED.locked_until
		RETURNING id, order_id, product_id, qty, price, added_by, locked_by, locked_until
	`, orderID, in.ProductID, in.Qty, productPrice, callerID).Scan(
		&item.ID, &item.OrderID, &item.ProductID,
		&item.Qty, &item.Price, &item.AddedBy,
		&item.LockedBy, &item.LockedUntil,
	)
	if err != nil {
		return OrderItem{}, fmt.Errorf("order.UpsertItem upsert: %w", err)
	}

	// 4. Rebuild snapshot + broadcast.
	if err := s.rebuildSnapshot(ctx, orderID, chatID, callerID); err != nil {
		// Non-fatal: log but don't fail the request.
		fmt.Printf("order.UpsertItem rebuildSnapshot: %v\n", err)
	}

	return item, nil
}

// RemoveItem removes a line from a draft order.
func (s *Service) RemoveItem(ctx context.Context, orderID, itemID, callerID uuid.UUID) error {
	var chatID uuid.UUID
	var status string
	err := s.db.QueryRow(ctx, `SELECT chat_id, status FROM orders WHERE id = $1`, orderID).
		Scan(&chatID, &status)
	if err == pgx.ErrNoRows {
		return errValidation("order not found")
	}
	if err != nil {
		return fmt.Errorf("order.RemoveItem: %w", err)
	}
	if status != StatusDraft {
		return errValidation("order is not in draft status")
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM order_items WHERE id = $1 AND order_id = $2`, itemID, orderID)
	if err != nil {
		return fmt.Errorf("order.RemoveItem delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("item not found")
	}

	return s.rebuildSnapshot(ctx, orderID, chatID, callerID)
}

// ──────────────────────────── CONFIRM ────────────────────────────────────────

// Confirm transitions an order from draft → confirmed.
// Checks daily limit from plans table (ТЗ раздел 3.1).
// Stock deduction is handled by the DB trigger (003_triggers.sql).
// Broadcasts order.confirmed to the chat.
func (s *Service) Confirm(ctx context.Context, orderID, callerID uuid.UUID, producerTZ string) (Order, error) {
	// 1. Load order.
	var o Order
	var producerID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT o.id, o.chat_id, o.status, o.total,
		       c.producer_id
		FROM   orders o
		JOIN   chats  c ON c.id = o.chat_id
		WHERE  o.id = $1
	`, orderID).Scan(&o.ID, &o.ChatID, &o.Status, &o.Total, &producerID)
	if err == pgx.ErrNoRows {
		return Order{}, errValidation("order not found")
	}
	if err != nil {
		return Order{}, fmt.Errorf("order.Confirm load: %w", err)
	}
	if o.Status != StatusDraft {
		return Order{}, errValidation("only draft orders can be confirmed")
	}

	// 2. Check daily limit for the producer's current plan (ТЗ раздел 3.1).
	if err := s.checkDailyLimit(ctx, producerID, producerTZ); err != nil {
		return Order{}, err
	}

	// 3. Confirm: update status + set confirmed_by/confirmed_at.
	// The DB trigger (fn_deduct_stock_on_confirm) fires here automatically.
	err = s.db.QueryRow(ctx, `
		UPDATE orders
		SET    status       = 'confirmed',
		       confirmed_by = $2,
		       confirmed_at = NOW(),
		       updated_at   = NOW()
		WHERE  id = $1
		RETURNING id, chat_id, status, created_by, confirmed_by, confirmed_at,
		          current_items_jsonb, total, created_at, updated_at
	`, orderID, callerID).Scan(
		&o.ID, &o.ChatID, &o.Status, &o.CreatedBy, &o.ConfirmedBy, &o.ConfirmedAt,
		&o.CurrentItemsJSON, &o.Total, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return Order{}, fmt.Errorf("order.Confirm update: %w", err)
	}

	// 4. Broadcast order.confirmed to the whole chat.
	if data, err := ws.NewEvent(ws.EventOrderConfirmed, ws.OrderConfirmedPayload{
		OrderID:     o.ID,
		ChatID:      o.ChatID,
		ConfirmedBy: callerID,
		Total:       o.Total,
	}); err == nil {
		encoded, _ := data.Encode()
		s.hub.Broadcast(ws.BroadcastMsg{ChatID: o.ChatID, Data: encoded})
	}

	return o, nil
}

// checkDailyLimit returns ErrDailyLimitReached if the producer has reached
// their plan's order_limit_per_day for today (in their timezone).
func (s *Service) checkDailyLimit(ctx context.Context, producerID uuid.UUID, tz string) error {
	if tz == "" {
		tz = "Asia/Tashkent"
	}

	var limitPerDay *int // NULL = unlimited
	err := s.db.QueryRow(ctx, `
		SELECT pl.order_limit_per_day
		FROM   subscriptions s
		JOIN   plans         pl ON pl.id = s.plan_id
		WHERE  s.producer_id = $1
		  AND  s.status      IN ('trial', 'active')
	`, producerID).Scan(&limitPerDay)
	if err == pgx.ErrNoRows {
		return nil // no subscription row → allow (shouldn't happen but be safe)
	}
	if err != nil {
		return fmt.Errorf("order.checkDailyLimit plan: %w", err)
	}
	if limitPerDay == nil {
		return nil // unlimited plan
	}

	// Count confirmed orders today in producer's timezone.
	var confirmedToday int
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM   orders  o
		JOIN   chats   c ON c.id = o.chat_id
		WHERE  c.producer_id = $1
		  AND  o.status      = 'confirmed'
		  AND  o.confirmed_at >= date_trunc('day', NOW() AT TIME ZONE $2) AT TIME ZONE $2
	`, producerID, tz).Scan(&confirmedToday)
	if err != nil {
		return fmt.Errorf("order.checkDailyLimit count: %w", err)
	}

	if confirmedToday >= *limitPerDay {
		return errDailyLimit(*limitPerDay)
	}
	return nil
}

// ──────────────────────────── REPEAT ─────────────────────────────────────────

// Repeat copies items from the last confirmed order into a new draft.
// Prices are taken from the CURRENT catalog (ТЗ раздел 4.6).
func (s *Service) Repeat(ctx context.Context, chatID, callerID uuid.UUID) (RepeatResult, error) {
	// 1. Find last confirmed order in this chat.
	var lastOrderID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT id FROM orders
		WHERE  chat_id = $1 AND status = 'confirmed'
		ORDER  BY confirmed_at DESC
		LIMIT  1
	`, chatID).Scan(&lastOrderID)
	if err == pgx.ErrNoRows {
		return RepeatResult{}, errValidation("no confirmed orders in this chat to repeat")
	}
	if err != nil {
		return RepeatResult{}, fmt.Errorf("order.Repeat find last: %w", err)
	}

	// 2. Load items from that order.
	rows, err := s.db.Query(ctx, `
		SELECT product_id, qty FROM order_items WHERE order_id = $1
	`, lastOrderID)
	if err != nil {
		return RepeatResult{}, fmt.Errorf("order.Repeat load items: %w", err)
	}
	defer rows.Close()

	type prevItem struct {
		ProductID uuid.UUID
		Qty       float64
	}
	var prevItems []prevItem
	for rows.Next() {
		var pi prevItem
		if err := rows.Scan(&pi.ProductID, &pi.Qty); err != nil {
			return RepeatResult{}, err
		}
		prevItems = append(prevItems, pi)
	}
	if err := rows.Err(); err != nil {
		return RepeatResult{}, err
	}

	// 3. Create new draft.
	draft, err := s.CreateDraft(ctx, chatID, callerID)
	if err != nil {
		return RepeatResult{}, err
	}

	// 4. Copy items — use CURRENT prices.
	var warnings []string
	for _, pi := range prevItems {
		var price float64
		var productName string
		err := s.db.QueryRow(ctx, `
			SELECT price, name FROM products WHERE id = $1 AND is_active = TRUE
		`, pi.ProductID).Scan(&price, &productName)
		if err == pgx.ErrNoRows {
			// ТЗ раздел 4.6: "если товар удалён — пропустить + warning"
			warnings = append(warnings, fmt.Sprintf("товар %s больше не доступен, пропущен", pi.ProductID))
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("ошибка при загрузке товара %s: %v", pi.ProductID, err))
			continue
		}

		// Check stock (ТЗ раздел 4.6: "если stock < qty — warning, но включить")
		var stockQty float64
		s.db.QueryRow(ctx, `SELECT stock_qty FROM products WHERE id = $1`, pi.ProductID).Scan(&stockQty)
		if stockQty < pi.Qty {
			warnings = append(warnings, fmt.Sprintf("%s: остаток (%g) ниже заказа (%g)", productName, stockQty, pi.Qty))
		}

		_, err = s.db.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, qty, price, added_by)
			VALUES ($1, $2, $3, $4, $5)
		`, draft.ID, pi.ProductID, pi.Qty, price, callerID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: не удалось добавить: %v", productName, err))
		}
	}

	// 5. Rebuild snapshot.
	if err := s.rebuildSnapshot(ctx, draft.ID, chatID, callerID); err != nil {
		warnings = append(warnings, "не удалось обновить снапшот: "+err.Error())
	}

	// 6. Reload draft with updated snapshot.
	draft, err = s.GetOrder(ctx, draft.ID)
	if err != nil {
		return RepeatResult{}, err
	}

	return RepeatResult{Order: draft, Warnings: warnings}, nil
}

// ──────────────────────────── SNAPSHOT ───────────────────────────────────────

// rebuildSnapshot recomputes current_items_jsonb and total for the order,
// then broadcasts order.item_updated to the chat.
// Called after every item mutation (ТЗ раздел 12.1).
func (s *Service) rebuildSnapshot(ctx context.Context, orderID, chatID, changedBy uuid.UUID) error {
	// Build snapshot from current order_items joined with products.
	rows, err := s.db.Query(ctx, `
		SELECT oi.id, oi.product_id, p.name, p.unit,
		       oi.qty, oi.price, oi.qty * oi.price AS subtotal
		FROM   order_items oi
		JOIN   products    p ON p.id = oi.product_id
		WHERE  oi.order_id = $1
		ORDER  BY oi.created_at
	`, orderID)
	if err != nil {
		return fmt.Errorf("rebuildSnapshot query: %w", err)
	}
	defer rows.Close()

	var (
		items []SnapshotItem
		total float64
	)
	for rows.Next() {
		var si SnapshotItem
		if err := rows.Scan(&si.ItemID, &si.ProductID, &si.Name, &si.Unit,
			&si.Qty, &si.Price, &si.Subtotal); err != nil {
			return err
		}
		items = append(items, si)
		total += si.Subtotal
	}
	if err := rows.Err(); err != nil {
		return err
	}

	snapshot, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("rebuildSnapshot marshal: %w", err)
	}

	// Persist snapshot + total.
	_, err = s.db.Exec(ctx, `
		UPDATE orders
		SET    current_items_jsonb = $2,
		       total               = $3,
		       updated_at          = NOW()
		WHERE  id = $1
	`, orderID, snapshot, total)
	if err != nil {
		return fmt.Errorf("rebuildSnapshot update: %w", err)
	}

	// Broadcast to chat (ExcludeID=uuid.Nil → everyone sees the update).
	payload := map[string]any{
		"order_id":   orderID,
		"changed_by": changedBy,
		"items":      items,
		"total":      total,
	}
	if ev, err := ws.NewEvent(ws.EventOrderItemUpdated, payload); err == nil {
		encoded, _ := ev.Encode()
		s.hub.Broadcast(ws.BroadcastMsg{ChatID: chatID, Data: encoded})
	}

	return nil
}

// ──────────────────────────── errors ─────────────────────────────────────────

type dailyLimitError struct{ limit int }

func (e dailyLimitError) Error() string {
	return fmt.Sprintf("DAILY_LIMIT_REACHED: дневной лимит %d заказов исчерпан", e.limit)
}

func errDailyLimit(limit int) error { return dailyLimitError{limit} }

// IsDailyLimitError returns true for daily limit errors (→ 403).
func IsDailyLimitError(err error) bool {
	_, ok := err.(dailyLimitError)
	return ok
}

// WriteOrderChange records one entry in order_changes (audit log).
func WriteOrderChange(ctx context.Context, db *pgxpool.Pool, orderID, changedBy uuid.UUID, field, oldVal, newVal string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO order_changes (order_id, field, old_val, new_val, changed_by)
		VALUES ($1, $2, $3, $4, $5)
	`, orderID, field, oldVal, newVal, changedBy)
	return err
}

// TashkentLocation returns the Asia/Tashkent timezone (used as fallback).
var TashkentLocation = mustLoadTZ("Asia/Tashkent")

func mustLoadTZ(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}
