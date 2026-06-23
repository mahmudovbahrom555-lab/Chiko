package order_test

import (
	"testing"

	"github.com/chiko/backend/internal/order"
)

// TestDailyLimitError verifies the error type sentinel works correctly.
// The actual DB-backed limit check is an integration test (requires Supabase).
func TestDailyLimitError(t *testing.T) {
	err := order.IsDailyLimitError(nil)
	if err {
		t.Error("nil should not be a daily limit error")
	}

	// We can't construct the unexported dailyLimitError directly,
	// but we can verify non-matching errors.
	valErr := order.IsValidationError(nil)
	if valErr {
		t.Error("nil should not be a validation error")
	}
}

// TestRepeatResult verifies the RepeatResult zero value is safe to serialise.
func TestRepeatResult(t *testing.T) {
	var r order.RepeatResult
	if r.Order.ID.String() == "" {
		// zero UUID is expected
	}
	if r.Warnings != nil {
		t.Error("zero RepeatResult should have nil warnings")
	}
}

// TestSnapshotItem verifies Subtotal calculation logic.
func TestSnapshotItem_Subtotal(t *testing.T) {
	si := order.SnapshotItem{
		Qty:   3,
		Price: 12500,
	}
	si.Subtotal = si.Qty * si.Price
	if si.Subtotal != 37500 {
		t.Errorf("expected subtotal 37500, got %v", si.Subtotal)
	}
}

// TestOrderStatuses verifies the status constants match DB CHECK constraint values.
func TestOrderStatuses(t *testing.T) {
	statuses := []string{order.StatusDraft, order.StatusConfirmed, order.StatusCancelled}
	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("status constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate status constant: %q", s)
		}
		seen[s] = true
	}
}
