package catalog

import (
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID         uuid.UUID `json:"id"`
	ProducerID uuid.UUID `json:"producer_id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
}

type Product struct {
	ID         uuid.UUID  `json:"id"`
	ProducerID uuid.UUID  `json:"producer_id"`
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
	Name       string     `json:"name"`
	Unit       string     `json:"unit"`
	Price      float64    `json:"price"`
	StockQty   float64    `json:"stock_qty"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateProductInput validates incoming product data.
type CreateProductInput struct {
	CategoryID *uuid.UUID `json:"category_id"`
	Name       string     `json:"name"`
	Unit       string     `json:"unit"`
	Price      float64    `json:"price"`
	StockQty   float64    `json:"stock_qty"`
}

func (c *CreateProductInput) Valid() error {
	if c.Name == "" {
		return errValidation("name is required")
	}
	if c.Price < 0 {
		return errValidation("price must be ≥ 0")
	}
	if c.StockQty < 0 {
		return errValidation("stock_qty must be ≥ 0")
	}
	return nil
}

type UpdateProductInput struct {
	CategoryID *uuid.UUID `json:"category_id"`
	Name       *string    `json:"name"`
	Unit       *string    `json:"unit"`
	Price      *float64   `json:"price"`
	StockQty   *float64   `json:"stock_qty"`
	IsActive   *bool      `json:"is_active"`
}

// SearchParams for catalog search endpoint.
type SearchParams struct {
	Query      string     `json:"q"`
	CategoryID *uuid.UUID `json:"category_id"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}

// ImportRow is one row from the Excel template.
type ImportRow struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Unit     string  `json:"unit"`
	StockQty float64 `json:"stock_qty"`
}

// ImportPreview is the response for POST /api/catalog/import (preview step).
type ImportPreview struct {
	Rows     []ImportRow `json:"rows"`
	Total    int         `json:"total"`
	Warnings []string    `json:"warnings,omitempty"`
}

// DefaultCategoryNames are the preset categories per ТЗ раздел 9.2.
var DefaultCategoryNames = []string{
	"Напитки",
	"Снеки",
	"Молочка",
	"Хлеб",
	"Бытовая химия",
	"Парфюмерия",
	"Игрушки",
	"Стройматериалы",
	"Другое",
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func errValidation(msg string) error { return validationError{msg} }

// IsValidationError returns true for user-input errors (→ 400).
func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
