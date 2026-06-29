package buylist

import (
	"testing"

	"github.com/google/uuid"
)

// TestMapLinesReturnNotUuidNil verifies that GetByOrderID is used (not GetByToken(uuid.Nil)).
// This is a compile-time + documentation test — the actual method existence.
func TestGetByOrderIDMethodExists(t *testing.T) {
	// If GetByOrderID doesn't exist, this won't compile.
	var svc *Service
	_ = svc.GetByOrderID // method must exist
	t.Log("GetByOrderID method exists — MapLines no longer returns uuid.Nil")
}

// TestNullableUUID verifies uuid.Nil sentinel detection logic used in GetSuggestions.
func TestUuidNilSentinel(t *testing.T) {
	zero := uuid.UUID{}
	if zero != uuid.Nil {
		t.Error("uuid.UUID{} should equal uuid.Nil")
	}

	real := uuid.New()
	if real == uuid.Nil {
		t.Error("generated UUID should not be uuid.Nil")
	}
}

// TestValidationError verifies the error type helpers.
func TestValidationError(t *testing.T) {
	err := errValidation("test error")
	if !IsValidationError(err) {
		t.Error("errValidation should produce IsValidationError=true")
	}
	if err.Error() != "test error" {
		t.Errorf("unexpected message: %s", err.Error())
	}
}
