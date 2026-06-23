package debt

import (
	"time"

	"github.com/google/uuid"
)

// Transaction types (ТЗ раздел 6.4 + раздел 7).
const (
	TypeDelivery          = "delivery"          // приход товара, sign=+1
	TypePayment           = "payment"            // оплата, sign=-1
	TypeReturnCorrection  = "return_correction"  // возврат-корректировка producer, sign=-1, confirmed сразу
	TypeCorrection        = "correction"         // исправление ошибки producer, INSERT only
)

// Tx statuses.
const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusDisputed  = "disputed"
)

// Return request statuses (ТЗ раздел 7).
const (
	ReturnPending   = "pending"
	ReturnAttention = "attention"  // SLA exceeded — требует внимания
	ReturnResolved  = "resolved"   // producer создал correction
	ReturnDisputed  = "disputed"   // client не согласен с correction
)

// Tx mirrors debt_transactions (append-only).
type Tx struct {
	ID            uuid.UUID  `json:"id"`
	ChatID        uuid.UUID  `json:"chat_id"`
	Type          string     `json:"type"`
	Amount        float64    `json:"amount"`
	Sign          int        `json:"sign"`
	InitiatorID   uuid.UUID  `json:"initiator_id"`
	ConfirmedByID *uuid.UUID `json:"confirmed_by_id,omitempty"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
	Status        string     `json:"status"`
	Comment       string     `json:"comment,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Balance is the response for GET /api/debt/balance/:chat_id.
// Formula: SUM(amount * sign) WHERE status IN ('pending', 'confirmed').
// Negative result = prepayment (credit).
type Balance struct {
	ChatID     uuid.UUID `json:"chat_id"`
	Balance    float64   `json:"balance"`   // может быть отрицательным (предоплата)
	Currency   string    `json:"currency"`  // из producers.catalog_currency
	HasPending bool      `json:"has_pending"`
}

// ReturnRequest mirrors return_requests.
type ReturnRequest struct {
	ID         uuid.UUID  `json:"id"`
	ChatID     uuid.UUID  `json:"chat_id"`
	OrderID    uuid.UUID  `json:"order_id"`
	ItemsJSON  []byte     `json:"items"`        // [{product_id, qty, reason}]
	PhotoURLs  []string   `json:"photo_urls"`
	Status     string     `json:"status"`
	Escalated  bool       `json:"escalated"`
	CreatedBy  uuid.UUID  `json:"created_by"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateDeliveryInput — producer initiates goods delivery (sign=+1).
type CreateDeliveryInput struct {
	ChatID  uuid.UUID `json:"chat_id"`
	Amount  float64   `json:"amount"`
	Comment string    `json:"comment"`
}

// CreatePaymentInput — either side initiates a payment (sign=-1).
type CreatePaymentInput struct {
	ChatID  uuid.UUID `json:"chat_id"`
	Amount  float64   `json:"amount"`
	Comment string    `json:"comment"`
}

// CreateReturnRequestInput — client reports a problem.
type CreateReturnRequestInput struct {
	ChatID     uuid.UUID `json:"chat_id"`
	OrderID    uuid.UUID `json:"order_id"`
	ItemsJSON  []byte    `json:"items"`
	PhotoURLs  []string  `json:"photo_urls"`
}

// CreateReturnCorrectionInput — producer resolves a return request.
type CreateReturnCorrectionInput struct {
	ReturnRequestID uuid.UUID `json:"return_request_id"`
	Amount          float64   `json:"amount"`
	Comment         string    `json:"comment"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func errValidation(msg string) error    { return validationError{msg} }

// IsValidationError returns true for user-input errors (→ 400).
func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
