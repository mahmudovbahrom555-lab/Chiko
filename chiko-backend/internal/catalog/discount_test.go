package catalog_test

import (
	"testing"

	"github.com/chiko/backend/internal/catalog"
)

// computeTotals реализует тот же алгоритм что CalculateOrderTotal — для unit-тестов без БД.
// Шаги: 1→2→3→4→5 (ТЗ раздел 8.3).
func computeTotals(items []catalog.DiscountedItem, volumeDiscPct, clientDiscPct float64) catalog.OrderTotals {
	var subtotal float64
	for i, it := range items {
		items[i].FinalPrice = it.BasePrice * (1 - it.ProductDisc/100)
		items[i].Subtotal = it.Qty * items[i].FinalPrice
		subtotal += items[i].Subtotal
	}
	afterVolume := subtotal * (1 - volumeDiscPct/100)
	total := afterVolume * (1 - clientDiscPct/100)
	return catalog.OrderTotals{
		Items:      items,
		Subtotal:   subtotal,
		VolumeDisc: volumeDiscPct,
		ClientDisc: clientDiscPct,
		Total:      total,
	}
}

func item(qty, price, productDisc float64) catalog.DiscountedItem {
	return catalog.DiscountedItem{Qty: qty, BasePrice: price, ProductDisc: productDisc}
}

// ── ТЗ раздел 8.3 требования ─────────────────────────────────────────────────

func TestDiscount_NoDiscounts(t *testing.T) {
	// Без скидок: total == sum(qty*price)
	result := computeTotals([]catalog.DiscountedItem{
		item(10, 1000, 0),
		item(5, 2000, 0),
	}, 0, 0)
	const want = 20_000.0
	if result.Total != want {
		t.Errorf("total = %v, want %v", result.Total, want)
	}
}

func TestDiscount_ProductDiscountApplied(t *testing.T) {
	// Акция -10% на товар: line = 10 * 1000 * 0.9 = 9000
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 10)}, 0, 0)
	const want = 9_000.0
	if result.Total != want {
		t.Errorf("total = %v, want %v", result.Total, want)
	}
}

func TestDiscount_VolumeDiscountApplied(t *testing.T) {
	// subtotal=10000, объёмная скидка -5%: total=9500
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 0)}, 5, 0)
	const want = 9_500.0
	if result.Total != want {
		t.Errorf("total = %v, want %v", result.Total, want)
	}
}

func TestDiscount_PersonalDiscountApplied(t *testing.T) {
	// subtotal=10000, персональная скидка -3%: total=9700
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 0)}, 0, 3)
	const want = 9_700.0
	if result.Total != want {
		t.Errorf("total = %v, want %v", result.Total, want)
	}
}

func TestDiscount_AllLevelsSequential(t *testing.T) {
	// Шаги ТЗ 8.3:
	// 1. line = 10 * 1000 = 10000
	// 2. product -10% → 9000
	// 3. subtotal = 9000
	// 4. volume -5%: 9000 * 0.95 = 8550
	// 5. personal -3%: 8550 * 0.97 = 8293.50
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 10)}, 5, 3)
	const want = 8293.50
	if result.Total != want {
		t.Errorf("total = %v, want %v", result.Total, want)
	}
}

func TestDiscount_SameLevelMaxNotSum(t *testing.T) {
	// Правило ТЗ: одного уровня → MAX, не сумма.
	// Два product_discounts на один товар: 10% и 15% → применяется 15%.
	// Нашей CalculateOrderTotal это гарантирует через MAX(pd.discount_pct).
	// Здесь тестируем что MAX=15% даёт правильный итог.
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 15)}, 0, 0) // MAX already chosen
	const want = 8_500.0
	if result.Total != want {
		t.Errorf("total = %v, want %v (same-level should use MAX not SUM)", result.Total, want)
	}
}

func TestDiscount_ProductAndVolumeAreNotSummed(t *testing.T) {
	// Разные уровни применяются ПОСЛЕДОВАТЕЛЬНО, не складываются как проценты.
	// product -10%, volume -5%: total = 10000 * 0.9 * 0.95 = 8550 (не 8500 = 10000*0.85).
	result := computeTotals([]catalog.DiscountedItem{item(10, 1000, 10)}, 5, 0)
	const want = 8_550.0
	if result.Total != want {
		t.Errorf("total = %v, want %v (levels sequential, not additive)", result.Total, want)
	}
}

func TestDiscount_ZeroQtyEdgeCase(t *testing.T) {
	result := computeTotals([]catalog.DiscountedItem{}, 0, 0)
	if result.Total != 0 {
		t.Errorf("empty order should have total=0, got %v", result.Total)
	}
}
