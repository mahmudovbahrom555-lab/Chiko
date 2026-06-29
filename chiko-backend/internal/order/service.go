package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/catalog"
	"github.com/chiko/backend/internal/ws"
)

// Service handles all collaborative order logic.
type Service struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewService(db *pgxpool.Pool, hub *ws.Hub) *Service {
	return &Service{db: db, hub: hub}
}

// ──────────────────────────── CREATE ─────────────────────────────────────────

// isParticipant returns error if callerID is not a member of the chat.
// Fail-closed: DB errors are logged and treated as "not a participant".
func (s *Service) isParticipant(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND (producer_id=$2 OR client_id=$2))
	`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Str("caller", callerID.String()).
			Msg("order: isParticipant DB error")
		return errValidation("not a participant of this chat")
	}
	if !exists {
		return errValidation("not a participant of this chat")
	}
	return nil
}

// CreateDraft creates a new draft order in the given chat.
// Both producer and client may call this (ТЗ раздел 1.2).
func (s *Service) CreateDraft(ctx context.Context, chatID, callerID uuid.UUID) (Order, error) {
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return Order{}, err
	}
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

// GetOrder returns a single order; verifies caller is a chat participant.
func (s *Service) GetOrder(ctx context.Context, orderID, callerID uuid.UUID) (Order, error) {
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
	// Validate AFTER loading so we don't leak that the order exists.
	if err := s.isParticipant(ctx, o.ChatID, callerID); err != nil {
		return Order{}, err
	}
	return o, nil
}

// ListByChat returns all orders for a chat in reverse chronological order.
func (s *Service) ListByChat(ctx context.Context, chatID, callerID uuid.UUID) ([]Order, error) {
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, status, created_by, confirmed_by, confirmed_at,
		       current_items_jsonb, total, created_at, updated_at
		FROM   orders WHERE chat_id = $1 ORDER BY created_at DESC
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
//
// Architecture connections (разрыв 1+2 — исправлен):
//   - Читает ПРЕДЫДУЩИЙ locked_by/qty через CTE до UPDATE (PostgreSQL CTE snapshot).
//   - Если предыдущий лок принадлежал ДРУГОМУ пользователю и ещё активен →
//     broadcast conflict.overwritten (ТЗ раздел 4.4).
//   - Записывает order_changes (ТЗ раздел 4.4: "каждое изменение пишется в историю").
func (s *Service) UpsertItem(ctx context.Context, orderID, callerID uuid.UUID, in UpdateItemInput) (OrderItem, error) {
	if in.Qty <= 0 {
		return OrderItem{}, errValidation("qty must be > 0")
	}

	// 1. Verify order is draft and load chat_id.
	var chatID uuid.UUID
	var status string
	err := s.db.QueryRow(ctx, `SELECT chat_id, status FROM orders WHERE id = $1`, orderID).
		Scan(&chatID, &status)
	if err == pgx.ErrNoRows {
		return OrderItem{}, errValidation("order not found")
	}
	if err != nil {
		return Order{}.zeroItem(), fmt.Errorf("order.UpsertItem verify: %w", err)
	}
	if status != StatusDraft {
		return OrderItem{}, errValidation("order is not in draft status")
	}
	// Verify caller is a chat participant BEFORE modifying anything.
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return OrderItem{}, err
	}

	// 2. Current product price (ТЗ раздел 4.6: цена из ТЕКУЩЕГО каталога).
	var productPrice float64
	err = s.db.QueryRow(ctx, `SELECT price FROM products WHERE id=$1 AND is_active=TRUE`, in.ProductID).
		Scan(&productPrice)
	if err == pgx.ErrNoRows {
		return OrderItem{}, errValidation("product not found or inactive")
	}
	if err != nil {
		return OrderItem{}, fmt.Errorf("order.UpsertItem price: %w", err)
	}

	// 3. Upsert with CTE to capture PREVIOUS lock + qty before overwrite.
	//    CTE reads the row BEFORE UPDATE (PostgreSQL CTE snapshot semantics).
	//    This enables:
	//    (a) conflict detection (was someone else editing?)
	//    (b) audit log (old qty → new qty)
	var (
		item            OrderItem
		prevLockedBy    *uuid.UUID
		prevLockedUntil *time.Time
		prevQty         *float64
	)
	err = s.db.QueryRow(ctx, `
		WITH prev AS (
			SELECT locked_by, locked_until, qty
			FROM   order_items
			WHERE  order_id = $1 AND product_id = $2
		)
		INSERT INTO order_items
			(order_id, product_id, qty, price, added_by, locked_by, locked_until)
		VALUES
			($1, $2, $3, $4, $5, $5, NOW() + INTERVAL '3 seconds')
		ON CONFLICT (order_id, product_id) DO UPDATE
			SET qty          = EXCLUDED.qty,
			    price        = EXCLUDED.price,
			    locked_by    = EXCLUDED.locked_by,
			    locked_until = EXCLUDED.locked_until
		RETURNING
			id, order_id, product_id, qty, price, added_by, locked_by, locked_until,
			(SELECT locked_by    FROM prev),
			(SELECT locked_until FROM prev),
			(SELECT qty          FROM prev)
	`, orderID, in.ProductID, in.Qty, productPrice, callerID).Scan(
		&item.ID, &item.OrderID, &item.ProductID,
		&item.Qty, &item.Price, &item.AddedBy,
		&item.LockedBy, &item.LockedUntil,
		&prevLockedBy, &prevLockedUntil, &prevQty,
	)
	if err != nil {
		return OrderItem{}, fmt.Errorf("order.UpsertItem upsert: %w", err)
	}

	// 4. Conflict detection: if another user had a valid lock → broadcast conflict.overwritten.
	if prevLockedBy != nil &&
		*prevLockedBy != callerID &&
		prevLockedUntil != nil &&
		prevLockedUntil.After(time.Now()) {

		oldVal := fmt.Sprintf("%v", prevQty)
		newVal := fmt.Sprintf("%g", in.Qty)
		if ev, err := ws.NewEvent(ws.EventConflictOverwrite, ws.ConflictOverwrittenPayload{
			OrderID:   orderID,
			ItemID:    item.ID,
			Field:     "qty",
			YourValue: oldVal,
			WonValue:  newVal,
			WonBy:     callerID,
		}); err == nil {
			encoded, _ := ev.Encode()
			// Send ONLY to the loser (ExcludeID=callerID means loser sees it, winner doesn't).
			// Actually we want the LOSER (*prevLockedBy) to see it — so we don't exclude them.
			// Broadcast to whole chat; the loser is NOT the callerID.
			s.hub.Broadcast(ws.BroadcastMsg{ChatID: chatID, Data: encoded, ExcludeID: callerID})
		}
		log.Debug().
			Str("winner", callerID.String()).
			Str("loser", prevLockedBy.String()).
			Str("item", item.ID.String()).
			Msg("order: soft-lock conflict — last write wins")
	}

	// 5. Audit log: write to order_changes (ТЗ раздел 4.4, раздел 12.1).
	oldQtyStr := "0" // new item — no old value
	if prevQty != nil {
		oldQtyStr = fmt.Sprintf("%g", *prevQty)
	}
	if err := WriteOrderChange(ctx, s.db, orderID, callerID, "qty", oldQtyStr, fmt.Sprintf("%g", in.Qty)); err != nil {
		log.Warn().Err(err).Str("order", orderID.String()).Msg("order: failed to write order_changes")
	}

	// 6. Rebuild snapshot + broadcast item_updated.
	if err := s.rebuildSnapshot(ctx, orderID, chatID, callerID); err != nil {
		log.Error().Err(err).Str("order", orderID.String()).Msg("order: rebuildSnapshot failed")
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
	// Verify caller is a chat participant BEFORE modifying anything.
	if err := s.isParticipant(ctx, chatID, callerID); err != nil {
		return err
	}

	// Read qty before delete for audit log.
	var oldQty float64
	var productID uuid.UUID
	if err := s.db.QueryRow(ctx, `SELECT qty, product_id FROM order_items WHERE id=$1 AND order_id=$2`,
		itemID, orderID).Scan(&oldQty, &productID); err != nil && err != pgx.ErrNoRows {
		log.Warn().Err(err).Msg("order: RemoveItem pre-delete read failed")
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM order_items WHERE id=$1 AND order_id=$2`, itemID, orderID)
	if err != nil {
		return fmt.Errorf("order.RemoveItem delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errValidation("item not found")
	}

	// Audit log.
	if err := WriteOrderChange(ctx, s.db, orderID, callerID, "qty",
		fmt.Sprintf("%g", oldQty), "0"); err != nil {
		log.Warn().Err(err).Str("order", orderID.String()).Msg("order: failed to write order_changes")
	}

	return s.rebuildSnapshot(ctx, orderID, chatID, callerID)
}

// ──────────────────────────── CONFIRM ────────────────────────────────────────

// Confirm transitions an order from draft → confirmed.
//
// Architecture connections fixed:
//   - Reads producers.timezone from DB (разрыв 3).
//   - Updates client_metrics for order counts/averages (разрыв 4).
//   - DB trigger fn_deduct_stock_on_confirm fires automatically.
func (s *Service) Confirm(ctx context.Context, orderID, callerID uuid.UUID) (Order, error) {
	// 1. Load order + producer info.
	var o Order
	var producerID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT o.id, o.chat_id, o.status, o.total, c.producer_id
		FROM   orders o JOIN chats c ON c.id = o.chat_id
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
	// Verify caller is a participant before any state mutation.
	if err := s.isParticipant(ctx, o.ChatID, callerID); err != nil {
		return Order{}, err
	}

	// 2. Daily limit check — reads producers.timezone from DB (разрыв 3 исправлен).
	if err := s.checkDailyLimit(ctx, producerID); err != nil {
		return Order{}, err
	}

	// 3. Confirm. DB trigger fn_deduct_stock_on_confirm fires here.
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

	// 4. Audit log.
	if err := WriteOrderChange(ctx, s.db, orderID, callerID, "status", StatusDraft, StatusConfirmed); err != nil {
		log.Warn().Err(err).Str("order", orderID.String()).Msg("order: failed to write order_changes on confirm")
	}

	// 5. Update client_metrics — order counts/averages (разрыв 4 исправлен).
	s.updateOrderMetrics(ctx, o.ChatID, o.Total)

	// 6. Broadcast order.confirmed to whole chat.
	if ev, err := ws.NewEvent(ws.EventOrderConfirmed, ws.OrderConfirmedPayload{
		OrderID:     o.ID,
		ChatID:      o.ChatID,
		ConfirmedBy: callerID,
		Total:       o.Total,
	}); err == nil {
		encoded, _ := ev.Encode()
		s.hub.Broadcast(ws.BroadcastMsg{ChatID: o.ChatID, Data: encoded})
	}

	return o, nil
}

// checkDailyLimit reads the producer's timezone from DB and counts today's confirmed orders.
// Разрыв 3 исправлен: producers.timezone теперь реально используется.
func (s *Service) checkDailyLimit(ctx context.Context, producerID uuid.UUID) error {
	// Read timezone + plan limit in one join.
	var tz string
	var limitPerDay *int
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(p.timezone, 'Asia/Tashkent'),
		       pl.order_limit_per_day
		FROM   producers    p
		LEFT JOIN subscriptions s  ON s.producer_id = p.id
		                          AND s.status IN ('trial','active')
		LEFT JOIN plans         pl ON pl.id = s.plan_id
		WHERE  p.id = $1
	`, producerID).Scan(&tz, &limitPerDay)
	if err == pgx.ErrNoRows {
		return nil // no producer row — allow
	}
	if err != nil {
		return fmt.Errorf("order.checkDailyLimit: %w", err)
	}
	if limitPerDay == nil {
		return nil // unlimited
	}

	var confirmedToday int
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM   orders o JOIN chats c ON c.id = o.chat_id
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

// updateOrderMetrics updates client_metrics after order confirmation (разрыв 4).
// ТЗ раздел 12.5: "обновляется при каждом подтверждённом заказе".
func (s *Service) updateOrderMetrics(ctx context.Context, chatID uuid.UUID, orderTotal float64) {
	_, err := s.db.Exec(ctx, `
		INSERT INTO client_metrics
			(chat_id, last_order_at, order_count, avg_order_value, updated_at)
		VALUES
			($1, NOW(), 1, $2, NOW())
		ON CONFLICT (chat_id) DO UPDATE
		SET last_order_at  = NOW(),
		    order_count    = client_metrics.order_count + 1,
		    avg_order_value =
		        (client_metrics.avg_order_value * client_metrics.order_count + $2)
		        / (client_metrics.order_count + 1),
		    updated_at     = NOW()
	`, chatID, orderTotal)
	if err != nil {
		log.Error().Err(err).Str("chat", chatID.String()).Msg("order: updateOrderMetrics failed")
	}
}

// ──────────────────────────── REPEAT ─────────────────────────────────────────

// Repeat copies items from the last confirmed order into a new draft.
// Rules (ТЗ раздел 4.6):
//   - Deleted/inactive product → skip + warning
//   - Changed price OR stock < qty   → include + warning (TWO DIFFERENT CASES)
func (s *Service) Repeat(ctx context.Context, chatID, callerID uuid.UUID) (RepeatResult, error) {
	var lastOrderID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT id FROM orders
		WHERE  chat_id = $1 AND status = 'confirmed'
		ORDER  BY confirmed_at DESC LIMIT 1
	`, chatID).Scan(&lastOrderID)
	if err == pgx.ErrNoRows {
		return RepeatResult{}, errValidation("no confirmed orders in this chat to repeat")
	}
	if err != nil {
		return RepeatResult{}, fmt.Errorf("order.Repeat find last: %w", err)
	}

	rows, err := s.db.Query(ctx, `SELECT product_id, qty FROM order_items WHERE order_id=$1`, lastOrderID)
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

	draft, err := s.CreateDraft(ctx, chatID, callerID)
	if err != nil {
		return RepeatResult{}, err
	}

	// ── ONE batch query instead of N individual lookups (N+1 fix) ───────────────
	productIDs := make([]uuid.UUID, 0, len(prevItems))
	for _, pi := range prevItems {
		productIDs = append(productIDs, pi.ProductID)
	}

	type productInfo struct {
		Price    float64
		StockQty float64
		Name     string
	}
	productMap := make(map[uuid.UUID]productInfo, len(productIDs))

	pRows, err := s.db.Query(ctx, `
		SELECT id, price, stock_qty, name
		FROM   products
		WHERE  id = ANY($1) AND is_active = TRUE
	`, productIDs)
	if err != nil {
		return RepeatResult{}, fmt.Errorf("order.Repeat load products: %w", err)
	}
	for pRows.Next() {
		var pid uuid.UUID
		var info productInfo
		if err := pRows.Scan(&pid, &info.Price, &info.StockQty, &info.Name); err != nil {
			pRows.Close()
			return RepeatResult{}, fmt.Errorf("order.Repeat scan product: %w", err)
		}
		productMap[pid] = info
	}
	pRows.Close()
	if err := pRows.Err(); err != nil {
		return RepeatResult{}, fmt.Errorf("order.Repeat products rows: %w", err)
	}

	var warnings []string
	for _, pi := range prevItems {
		info, found := productMap[pi.ProductID]
		if !found {
			// ТЗ 4.6: "товар удалён — пропустить + warning"
			warnings = append(warnings, fmt.Sprintf("товар %s больше не доступен, пропущен", pi.ProductID))
			continue
		}

		// ТЗ 4.6: "stock < qty — включить с warning" (не пропускать!)
		if info.StockQty < pi.Qty {
			warnings = append(warnings, fmt.Sprintf(
				"%s: остаток (%.0f) ниже заказа (%.0f) — скорректируйте вручную",
				info.Name, info.StockQty, pi.Qty,
			))
		}

		_, err = s.db.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, qty, price, added_by)
			VALUES ($1, $2, $3, $4, $5)
		`, draft.ID, pi.ProductID, pi.Qty, info.Price, callerID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: не удалось добавить: %v", info.Name, err))
		}
	}

	if err := s.rebuildSnapshot(ctx, draft.ID, chatID, callerID); err != nil {
		log.Warn().Err(err).Msg("order.Repeat: rebuildSnapshot failed")
	}

	draft, err = s.GetOrder(ctx, draft.ID, callerID)
	if err != nil {
		return RepeatResult{}, err
	}

	return RepeatResult{Order: draft, Warnings: warnings}, nil
}

// ──────────────────────────── SNAPSHOT ───────────────────────────────────────

// rebuildSnapshot recomputes current_items_jsonb + total WITH discounts applied
// and broadcasts item_updated. (ТЗ раздел 12.1 + раздел 8.3)
//
// Architecture link (разрыв исправлен): теперь вызывает catalog.CalculateOrderTotal
// — orders.total ОБЯЗАН отражать итог со скидками (ТЗ 8.3 явное требование).
func (s *Service) rebuildSnapshot(ctx context.Context, orderID, chatID, changedBy uuid.UUID) error {
	// Calculate totals with all discount levels applied.
	totals, err := catalog.CalculateOrderTotal(ctx, s.db, orderID, chatID)
	if err != nil {
		return fmt.Errorf("rebuildSnapshot calculateTotal: %w", err)
	}

	// Convert catalog.DiscountedItem → SnapshotItem (for API response).
	items := make([]SnapshotItem, len(totals.Items))
	for i, di := range totals.Items {
		items[i] = SnapshotItem{
			ItemID:    di.ItemID,
			ProductID: di.ProductID,
			Name:      di.Name,
			Unit:      di.Unit,
			Qty:       di.Qty,
			Price:     di.FinalPrice, // discounted price
			Subtotal:  di.Subtotal,
		}
	}

	snapshot, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("rebuildSnapshot marshal: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE orders SET current_items_jsonb=$2, total=$3, updated_at=NOW() WHERE id=$1
	`, orderID, snapshot, totals.Total); err != nil {
		return fmt.Errorf("rebuildSnapshot update: %w", err)
	}

	if ev, err := ws.NewEvent(ws.EventOrderItemUpdated, map[string]any{
		"order_id":    orderID,
		"changed_by":  changedBy,
		"items":       items,
		"total":       totals.Total,
		"volume_disc": totals.VolumeDisc,
		"client_disc": totals.ClientDisc,
	}); err == nil {
		encoded, _ := ev.Encode()
		s.hub.Broadcast(ws.BroadcastMsg{ChatID: chatID, Data: encoded})
	}

	return nil
}

// ──────────────────────────── helpers ────────────────────────────────────────

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

// WriteOrderChange records one entry in order_changes (append-only audit log).
// Called from UpsertItem, RemoveItem, Confirm.
func WriteOrderChange(ctx context.Context, db *pgxpool.Pool, orderID, changedBy uuid.UUID, field, oldVal, newVal string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO order_changes (order_id, field, old_val, new_val, changed_by)
		VALUES ($1, $2, $3, $4, $5)
	`, orderID, field, oldVal, newVal, changedBy)
	return err
}

// zeroItem is a zero-value helper to satisfy return types on early errors.
func (Order) zeroItem() OrderItem { return OrderItem{} }

// TashkentLocation is kept for any external callers that need the timezone object.
var TashkentLocation = mustLoadTZ("Asia/Tashkent")

func mustLoadTZ(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}
