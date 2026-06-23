package ws

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// EventType identifies what happened.
type EventType string

const (
	// Order events
	EventOrderItemUpdated  EventType = "order.item_updated"
	EventOrderItemLocked   EventType = "order.item_locked"
	EventOrderConfirmed    EventType = "order.confirmed"
	EventOrderCancelled    EventType = "order.cancelled"
	EventConflictOverwrite EventType = "conflict.overwritten"

	// Chat events
	EventMessageNew EventType = "message.new"

	// Debt events
	EventDebtCreated   EventType = "debt.created"
	EventDebtConfirmed EventType = "debt.confirmed"
	EventDebtDisputed  EventType = "debt.disputed"

	// System
	EventError EventType = "error"
)

// Event is the wire format for every WebSocket message.
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Encode serialises an Event to JSON bytes ready to send over the wire.
func (e Event) Encode() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("event.Encode: %w", err)
	}
	return b, nil
}

// MustEncode panics on error — use only for compile-time-known payloads.
func MustEncode(t EventType, payload any) []byte {
	p, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("ws.MustEncode: %v", err))
	}
	b, err := Event{Type: t, Payload: p}.Encode()
	if err != nil {
		panic(fmt.Sprintf("ws.MustEncode: %v", err))
	}
	return b
}

// NewEvent creates an Event from any JSON-serialisable payload.
func NewEvent(t EventType, payload any) (Event, error) {
	p, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("ws.NewEvent marshal: %w", err)
	}
	return Event{Type: t, Payload: p}, nil
}

// --- Typed payloads ---

type OrderItemUpdatedPayload struct {
	OrderID   uuid.UUID `json:"order_id"`
	ItemID    uuid.UUID `json:"item_id"`
	ProductID uuid.UUID `json:"product_id"`
	Qty       float64   `json:"qty"`
	Price     float64   `json:"price"`
	ChangedBy uuid.UUID `json:"changed_by"`
}

type OrderItemLockedPayload struct {
	OrderID   uuid.UUID `json:"order_id"`
	ItemID    uuid.UUID `json:"item_id"`
	LockedBy  uuid.UUID `json:"locked_by"`
}

type ConflictOverwrittenPayload struct {
	OrderID   uuid.UUID `json:"order_id"`
	ItemID    uuid.UUID `json:"item_id"`
	Field     string    `json:"field"`
	YourValue any       `json:"your_value"`
	WonValue  any       `json:"won_value"`
	WonBy     uuid.UUID `json:"won_by"`
}

type OrderConfirmedPayload struct {
	OrderID     uuid.UUID `json:"order_id"`
	ChatID      uuid.UUID `json:"chat_id"`
	ConfirmedBy uuid.UUID `json:"confirmed_by"`
	Total       float64   `json:"total"`
}

type MessageNewPayload struct {
	MessageID uuid.UUID `json:"message_id"`
	ChatID    uuid.UUID `json:"chat_id"`
	AuthorID  uuid.UUID `json:"author_id"`
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	VoiceURL  string    `json:"voice_url,omitempty"`
}

type DebtPayload struct {
	TxID    uuid.UUID `json:"tx_id"`
	ChatID  uuid.UUID `json:"chat_id"`
	Type    string    `json:"type"`
	Amount  float64   `json:"amount"`
	Sign    int       `json:"sign"`
	Balance float64   `json:"balance"` // пересчитанный баланс
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
