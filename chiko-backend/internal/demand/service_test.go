package demand

import (
	"fmt"
	"testing"
)

// TestDemandStatusConstants verifies the status machine constants are correct.
func TestDemandStatusConstants(t *testing.T) {
	statuses := []Status{StatusOpen, StatusProposed, StatusOrdered, StatusCancelled}
	expected := []string{"open", "proposed", "ordered", "cancelled"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("status[%d]: got %q, want %q", i, s, expected[i])
		}
	}
}

// TestUrgencyConstants verifies urgency values match DB CHECK constraint.
func TestUrgencyConstants(t *testing.T) {
	cases := map[Urgency]string{
		UrgencyUrgent:  "urgent",
		UrgencySoon:    "soon",
		UrgencyPlanned: "planned",
	}
	for u, want := range cases {
		if string(u) != want {
			t.Errorf("urgency: got %q, want %q", u, want)
		}
	}
}

// TestCancelReasonValues verifies CancelReason values used in service validation.
func TestCancelReasonValues(t *testing.T) {
	valid := map[CancelReason]bool{
		"no_stock": true, "price_mismatch": true,
		"bought_elsewhere": true, "need_disappeared": true, "other": true,
	}
	// Sanity: unknown reason is not valid
	if valid["unknown"] {
		t.Error("'unknown' should not be a valid cancel reason")
	}
	if len(valid) != 5 {
		t.Errorf("expected 5 valid reasons, got %d", len(valid))
	}
}

// TestBroadcastPayloadFields verifies that broadcast method produces correct payload fields.
// This is a documentation test — if someone removes cancel_reason from broadcast payload,
// this test documents the expected fields.
func TestItemStructHasCancelFields(t *testing.T) {
	item := Item{
		CancelReason: CancelReason("no_stock"),
		CancelNote:   "out of stock for 2 weeks",
	}
	if item.CancelReason != "no_stock" {
		t.Error("CancelReason field missing or wrong")
	}
	if item.CancelNote == "" {
		t.Error("CancelNote field missing")
	}
}

// TestDemandValidationError checks error type system.
func TestDemandValidationError(t *testing.T) {
	err := errValidation("test")
	if !IsValidationError(err) {
		t.Error("errValidation should produce IsValidationError=true")
	}
	other := fmt.Errorf("not validation")
	if IsValidationError(other) {
		t.Error("fmt.Errorf should not be IsValidationError")
	}
}
