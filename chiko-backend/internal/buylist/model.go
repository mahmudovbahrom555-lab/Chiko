package buylist

import (
	"time"

	"github.com/google/uuid"
)

// BuyList представляет orders с status='guest_buy_list'.
type BuyList struct {
	ID             uuid.UUID  `json:"id"`
	GuestToken     uuid.UUID  `json:"guest_token"`
	ProducerPhone  string     `json:"producer_phone,omitempty"`
	GuestPhone     string     `json:"guest_phone,omitempty"`
	Status         string     `json:"status"` // guest_buy_list | draft
	Lines          []Line     `json:"lines"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Line — одна строка Buy List (может быть несопоставленной с каталогом).
type Line struct {
	ID        uuid.UUID  `json:"id"`
	RawText   string     `json:"raw_text"`             // исходный текст от магазина
	ProductID *uuid.UUID `json:"product_id,omitempty"` // nil = не сопоставлено
	ProductName string   `json:"product_name,omitempty"`
	Qty       float64    `json:"qty"`
	Price     float64    `json:"price,omitempty"`      // 0 пока не сопоставлено
}

// SuggestedMatch — вариант из каталога для сопоставления строки.
type SuggestedMatch struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Unit        string    `json:"unit"`
	StockQty    float64   `json:"stock_qty"`
	Score       float64   `json:"score"`
	IsCached    bool      `json:"is_cached"` // из buy_list_mappings
}

// LineSuggestion — строка + предложенные сопоставления.
type LineSuggestion struct {
	Line    Line             `json:"line"`
	Matches []SuggestedMatch `json:"matches"`
}

// CreateInput — данные для создания Buy List (POST /api/buy-list).
type CreateInput struct {
	ProducerPhone string      `json:"producer_phone"` // E.164, кому отправляем
	GuestPhone    string      `json:"guest_phone"`    // номер магазина (опционально)
	Lines         []LineInput `json:"lines"`
}

type LineInput struct {
	Text string  `json:"text"` // свободный текст ("масло 20")
	Qty  float64 `json:"qty"`
}

// MapInput — выбор производителя: raw строка → product.
type MapInput struct {
	LineID    uuid.UUID `json:"line_id"`
	ProductID uuid.UUID `json:"product_id"` // uuid.Nil = "нет товара"
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func errValidation(msg string) error { return validationError{msg} }

func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
