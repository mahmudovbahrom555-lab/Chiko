package demand

import (
	"time"

	"github.com/google/uuid"
)

// Urgency задаётся розницей — производитель видит что нужно срочно.
type Urgency string

const (
	UrgencyUrgent  Urgency = "urgent"  // 🔥 нужно сегодня-завтра
	UrgencySoon    Urgency = "soon"    // 🟡 в течение недели
	UrgencyPlanned Urgency = "planned" // ⚪ планово, не горит
)

// Item — одна позиция в списке "что нужно заказать".
type Item struct {
	ID         uuid.UUID `json:"id"`
	ChatID     uuid.UUID `json:"chat_id"`
	CreatedBy  uuid.UUID `json:"created_by"`
	Name       string    `json:"name"`
	Qty        float64   `json:"qty"`
	Unit       string    `json:"unit"`
	Note       string    `json:"note,omitempty"`
	Urgency    Urgency   `json:"urgency"`
	IsIncluded bool      `json:"is_included"` // производитель добавил в черновик
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ProductMatch — один вариант товара из каталога производителя,
// предлагаемый для сопоставления с позицией спроса.
type ProductMatch struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Unit        string    `json:"unit"`
	StockQty    float64   `json:"stock_qty"`
	Score       float64   `json:"score"` // 0-1, сходство имён
}

// DemandSuggestion — позиция спроса + список вариантов из каталога.
// Производитель выбирает нужный вариант или оставляет пустым.
type DemandSuggestion struct {
	DemandItem Item           `json:"demand_item"`
	Matches    []ProductMatch `json:"matches"` // пустой = нет совпадений
}

// Mapping — выбор производителя: какой товар из каталога соответствует
// какой позиции спроса.
type Mapping struct {
	DemandItemID uuid.UUID `json:"demand_item_id"`
	ProductID    uuid.UUID `json:"product_id"`
}

type CreateInput struct {
	ChatID  uuid.UUID `json:"chat_id"`
	Name    string    `json:"name"`
	Qty     float64   `json:"qty"`
	Unit    string    `json:"unit"`
	Note    string    `json:"note"`
	Urgency Urgency   `json:"urgency"`
}

type UpdateInput struct {
	Name       *string  `json:"name"`
	Qty        *float64 `json:"qty"`
	Unit       *string  `json:"unit"`
	Note       *string  `json:"note"`
	Urgency    *Urgency `json:"urgency"`
	IsIncluded *bool    `json:"is_included"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func errValidation(msg string) error { return validationError{msg} }

func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
