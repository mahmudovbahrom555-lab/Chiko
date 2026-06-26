package demand

import (
	"time"

	"github.com/google/uuid"
)

// Status отслеживает жизненный цикл позиции спроса.
// open      → ждёт предложения от производителя
// proposed  → производитель включил в черновик заказа
// ordered   → заказ подтверждён
// cancelled → позиция отменена (остаётся видимой с badge "Отменено")
type Status string

const (
	StatusOpen      Status = "open"
	StatusProposed  Status = "proposed"
	StatusOrdered   Status = "ordered"
	StatusCancelled Status = "cancelled"
)

// Urgency задаётся розницей — производитель видит что нужно срочно.
// UI показывает: urgent=🔥 Срочно, soon=🟡 На этой неделе, planned=⚪ Планово.
type Urgency string

const (
	UrgencyUrgent  Urgency = "urgent"
	UrgencySoon    Urgency = "soon"
	UrgencyPlanned Urgency = "planned"
)

// Item — одна позиция в списке "что нужно заказать".
type Item struct {
	ID        uuid.UUID `json:"id"`
	ChatID    uuid.UUID `json:"chat_id"`
	CreatedBy uuid.UUID `json:"created_by"`
	Name      string    `json:"name"`
	Qty       float64   `json:"qty"`
	Unit      string    `json:"unit"`
	Note      string    `json:"note,omitempty"`
	Urgency   Urgency   `json:"urgency"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProductMatch — вариант из каталога производителя для сопоставления.
// IsPreferred=true если производитель уже выбирал этот товар для данного
// названия в этом чате (из demand_preferences).
type ProductMatch struct {
	ProductID   uuid.UUID `json:"product_id"`
	ProductName string    `json:"product_name"`
	Price       float64   `json:"price"`
	Unit        string    `json:"unit"`
	StockQty    float64   `json:"stock_qty"`
	Score       float64   `json:"score"`       // 0-1, сходство имён
	IsPreferred bool      `json:"is_preferred"` // был выбран раньше — показывать первым
}

// DemandSuggestion — позиция спроса + варианты из каталога.
// preferred вариант (если есть) всегда первый в списке.
type DemandSuggestion struct {
	DemandItem Item           `json:"demand_item"`
	Matches    []ProductMatch `json:"matches"`
}

// Mapping — явный выбор производителя: demand_item → product из каталога.
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
	Name    *string  `json:"name"`
	Qty     *float64 `json:"qty"`
	Unit    *string  `json:"unit"`
	Note    *string  `json:"note"`
	Urgency *Urgency `json:"urgency"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }

func errValidation(msg string) error { return validationError{msg} }

func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
