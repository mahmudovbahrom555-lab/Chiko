package order

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Status values for orders (ТЗ раздел 4, статусы: draft → confirmed | cancelled).
const (
	StatusDraft     = "draft"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

// Order mirrors the orders table.
type Order struct {
	ID               uuid.UUID       `json:"id"`
	ChatID           uuid.UUID       `json:"chat_id"`
	Status           string          `json:"status"`
	CreatedBy        uuid.UUID       `json:"created_by"`
	ConfirmedBy      *uuid.UUID      `json:"confirmed_by,omitempty"`
	ConfirmedAt      *time.Time      `json:"confirmed_at,omitempty"`
	CurrentItemsJSON json.RawMessage `json:"items"` // денормализованный снапшот
	Total            float64         `json:"total"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// OrderItem mirrors the order_items table.
type OrderItem struct {
	ID          uuid.UUID  `json:"id"`
	OrderID     uuid.UUID  `json:"order_id"`
	ProductID   uuid.UUID  `json:"product_id"`
	Qty         float64    `json:"qty"`
	Price       float64    `json:"price"`
	AddedBy     uuid.UUID  `json:"added_by"`
	LockedBy    *uuid.UUID `json:"locked_by,omitempty"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
}

// SnapshotItem is one entry in current_items_jsonb.
type SnapshotItem struct {
	ItemID    uuid.UUID `json:"item_id"`
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	Unit      string    `json:"unit"`
	Qty       float64   `json:"qty"`
	Price     float64   `json:"price"`
	Subtotal  float64   `json:"subtotal"`
}

// UpdateItemInput is the body for PUT /api/orders/:id/items/:item_id.
type UpdateItemInput struct {
	ProductID uuid.UUID `json:"product_id"` // required when adding a new item
	Qty       float64   `json:"qty"`
}

// RepeatResult is the response for POST /api/orders/:id/repeat.
type RepeatResult struct {
	Order    Order    `json:"order"`
	Warnings []string `json:"warnings,omitempty"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func errValidation(msg string) error    { return validationError{msg} }

// IsValidationError returns true for user-input errors (→ 400).
func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
