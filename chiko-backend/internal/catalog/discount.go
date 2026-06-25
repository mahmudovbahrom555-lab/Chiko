package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscountedItem is one line in the calculated order (with applied discounts).
type DiscountedItem struct {
	ItemID      uuid.UUID `json:"item_id"`
	ProductID   uuid.UUID `json:"product_id"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Qty         float64   `json:"qty"`
	BasePrice   float64   `json:"base_price"`
	FinalPrice  float64   `json:"final_price"`  // price after product discount
	Subtotal    float64   `json:"subtotal"`      // qty * FinalPrice
	ProductDisc float64   `json:"product_disc"`  // % applied at line level
}

// OrderTotals holds the full discount breakdown for an order.
type OrderTotals struct {
	Items      []DiscountedItem `json:"items"`
	Subtotal   float64          `json:"subtotal"`    // sum of line subtotals
	VolumeDisc float64          `json:"volume_disc"` // % applied to subtotal
	ClientDisc float64          `json:"client_disc"` // % applied after volume
	Total      float64          `json:"total"`       // final order total
}

// CalculateOrderTotal applies the 6-step discount algorithm from ТЗ раздел 8.3.
//
// Algorithm (CRITICAL — covered by tests):
//  1. line = qty × price
//  2. apply product discount (акция, only if valid_until > NOW() or NULL)
//  3. subtotal = sum of discounted lines
//  4. find best volume tier by subtotal or qty → apply MAX
//  5. apply personal client discount to result of step 4
//  6. total is fixed — no recalculation after Confirm
//
// Rule: same-level discounts → MAX, not sum.
// Different levels → sequential application.
func CalculateOrderTotal(ctx context.Context, db *pgxpool.Pool, orderID, chatID uuid.UUID) (OrderTotals, error) {
	// ── Step 1+2: load items with active product discounts ────────────────────
	rows, err := db.Query(ctx, `
		SELECT
			oi.id,
			oi.product_id,
			p.name,
			p.unit,
			oi.qty,
			oi.price,
			COALESCE(
				(SELECT MAX(pd.discount_pct)
				 FROM   product_discounts pd
				 WHERE  pd.product_id = oi.product_id
				   AND (pd.valid_until IS NULL OR pd.valid_until > NOW())
				), 0
			) AS product_disc_pct
		FROM   order_items oi
		JOIN   products p ON p.id = oi.product_id
		WHERE  oi.order_id = $1
		ORDER  BY oi.created_at
	`, orderID)
	if err != nil {
		return OrderTotals{}, fmt.Errorf("discount.CalculateOrderTotal items: %w", err)
	}
	defer rows.Close()

	var (
		items    []DiscountedItem
		subtotal float64
	)
	for rows.Next() {
		var item DiscountedItem
		var productDiscPct float64
		if err := rows.Scan(&item.ItemID, &item.ProductID, &item.Name, &item.Unit,
			&item.Qty, &item.BasePrice, &productDiscPct); err != nil {
			return OrderTotals{}, err
		}
		item.ProductDisc = productDiscPct
		item.FinalPrice = item.BasePrice * (1 - productDiscPct/100)
		item.Subtotal = item.Qty * item.FinalPrice
		items = append(items, item)
		subtotal += item.Subtotal
	}
	if err := rows.Err(); err != nil {
		return OrderTotals{}, err
	}
	if len(items) == 0 {
		return OrderTotals{Items: items}, nil
	}

	// ── Step 4: volume tiers ──────────────────────────────────────────────────
	// Find the producer via chat, then load tiers.
	var producerID uuid.UUID
	if err := db.QueryRow(ctx,
		`SELECT producer_id FROM chats WHERE id = $1`, chatID,
	).Scan(&producerID); err != nil {
		return OrderTotals{}, fmt.Errorf("discount.CalculateOrderTotal chat: %w", err)
	}

	// totalQty for quantity-based tiers (all items combined).
	var totalQty float64
	for _, it := range items {
		totalQty += it.Qty
	}

	var volumeDiscPct float64
	err = db.QueryRow(ctx, `
		SELECT COALESCE(MAX(discount_pct), 0)
		FROM   volume_tiers
		WHERE  producer_id = $1
		  AND (
		       (type = 'amount'   AND threshold <= $2)
		    OR (type = 'quantity' AND threshold <= $3)
		  )
	`, producerID, subtotal, totalQty).Scan(&volumeDiscPct)
	if err != nil {
		return OrderTotals{}, fmt.Errorf("discount.CalculateOrderTotal volume: %w", err)
	}

	afterVolume := subtotal * (1 - volumeDiscPct/100)

	// ── Step 5: personal client discount ─────────────────────────────────────
	var clientDiscPct float64
	db.QueryRow(ctx, `
		SELECT COALESCE(discount_pct, 0)
		FROM   client_discounts
		WHERE  chat_id = $1
	`, chatID).Scan(&clientDiscPct)

	total := afterVolume * (1 - clientDiscPct/100)

	return OrderTotals{
		Items:      items,
		Subtotal:   subtotal,
		VolumeDisc: volumeDiscPct,
		ClientDisc: clientDiscPct,
		Total:      total,
	}, nil
}
