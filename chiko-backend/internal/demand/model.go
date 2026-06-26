package demand

import (
	"time"

	"github.com/google/uuid"
)

// Item is one line in a chat's demand list.
type Item struct {
	ID        uuid.UUID  `json:"id"`
	ChatID    uuid.UUID  `json:"chat_id"`
	CreatedBy uuid.UUID  `json:"created_by"`
	Name      string     `json:"name"`
	Qty       float64    `json:"qty"`
	Unit      string     `json:"unit"`
	Note      string     `json:"note,omitempty"`
	IsFilled  bool       `json:"is_filled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type CreateInput struct {
	ChatID uuid.UUID `json:"chat_id"`
	Name   string    `json:"name"`
	Qty    float64   `json:"qty"`
	Unit   string    `json:"unit"`
	Note   string    `json:"note"`
}

type UpdateInput struct {
	Name     *string  `json:"name"`
	Qty      *float64 `json:"qty"`
	Unit     *string  `json:"unit"`
	Note     *string  `json:"note"`
	IsFilled *bool    `json:"is_filled"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func errValidation(msg string) error { return validationError{msg} }

func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
