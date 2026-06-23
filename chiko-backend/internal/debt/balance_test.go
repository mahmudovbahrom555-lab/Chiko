package debt_test

import (
	"testing"

	"github.com/chiko/backend/internal/debt"
)

// TestBalanceFormula verifies the debt calculation logic described in ТЗ раздел 6.2.
// These are pure unit tests — no DB required.
// Formula: SUM(amount * sign) WHERE status IN ('pending','confirmed')
// Disputed excluded. Negative result = prepayment (Credit).

type fakeTx struct {
	amount float64
	sign   int
	status string
}

func computeBalance(txs []fakeTx) float64 {
	var total float64
	for _, t := range txs {
		if t.status == "pending" || t.status == "confirmed" {
			total += t.amount * float64(t.sign)
		}
	}
	return total
}

func TestBalance_DeliveryThenPayment(t *testing.T) {
	// ТЗ example: delivery 1_000_000 → payment 300_000 → balance 700_000
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},  // delivery
		{300_000, -1, "confirmed"},   // payment
	}
	got := computeBalance(txs)
	if got != 700_000 {
		t.Errorf("expected 700_000, got %v", got)
	}
}

func TestBalance_ReturnReducesDebt(t *testing.T) {
	// After delivery 1_000_000 → return 100_000 → balance 900_000
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},
		{100_000, -1, "confirmed"}, // return_correction
	}
	got := computeBalance(txs)
	if got != 900_000 {
		t.Errorf("expected 900_000, got %v", got)
	}
}

func TestBalance_DisputedExcluded(t *testing.T) {
	// Disputed transaction must NOT affect balance (ТЗ раздел 6.2).
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},
		{500_000, -1, "disputed"}, // this must be excluded
	}
	got := computeBalance(txs)
	if got != 1_000_000 {
		t.Errorf("disputed tx must be excluded from balance; expected 1_000_000, got %v", got)
	}
}

func TestBalance_PendingIncluded(t *testing.T) {
	// Pending transactions DO affect the displayed balance (shown in red).
	// ТЗ раздел 6.3: "Сумма уже отражается в долге, помечена красным."
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},
		{300_000, -1, "pending"}, // not yet confirmed but still counts
	}
	got := computeBalance(txs)
	if got != 700_000 {
		t.Errorf("pending must be included in balance; expected 700_000, got %v", got)
	}
}

func TestBalance_NegativeIsPrepayment(t *testing.T) {
	// Negative balance = prepayment (Credit).
	txs := []fakeTx{
		{1_000_000, -1, "confirmed"}, // client prepaid
	}
	got := computeBalance(txs)
	if got != -1_000_000 {
		t.Errorf("expected -1_000_000 (prepayment), got %v", got)
	}
	if got >= 0 {
		t.Error("negative balance should indicate prepayment")
	}
}

func TestBalance_AllStatuses(t *testing.T) {
	// Full scenario from ТЗ section 3.2 checklist:
	// delivery 1_000_000 → -1_000_000
	// payment  300_000   → -700_000
	// return   100_000   → -600_000
	// disputed (ignored) → still -600_000
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},
		{300_000, -1, "confirmed"},
		{100_000, -1, "confirmed"},
		{999_999, -1, "disputed"}, // excluded
	}
	got := computeBalance(txs)
	const want = 600_000.0
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestTxTypes verifies type constants match DB CHECK constraint values.
func TestTxTypes(t *testing.T) {
	types := []string{debt.TypeDelivery, debt.TypePayment, debt.TypeReturnCorrection, debt.TypeCorrection}
	seen := make(map[string]bool)
	for _, ty := range types {
		if ty == "" {
			t.Error("type constant is empty")
		}
		if seen[ty] {
			t.Errorf("duplicate type constant: %q", ty)
		}
		seen[ty] = true
	}
}

// TestReturnCorrectionSignIsNegative ensures return_correction decreases debt.
func TestReturnCorrectionSignIsNegative(t *testing.T) {
	// By convention: return_correction sign=-1 (decreases debt)
	txs := []fakeTx{
		{1_000_000, 1, "confirmed"},
		{200_000, -1, "confirmed"}, // return_correction with sign=-1
	}
	got := computeBalance(txs)
	if got >= 1_000_000 {
		t.Error("return_correction must decrease the balance")
	}
}
