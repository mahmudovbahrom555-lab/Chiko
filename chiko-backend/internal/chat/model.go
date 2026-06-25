package chat

import (
	"time"

	"github.com/google/uuid"
)

// Chat represents a producer↔client pair (ТЗ раздел 12.7).
type Chat struct {
	ID                 uuid.UUID  `json:"id"`
	ProducerID         uuid.UUID  `json:"producer_id"`
	ClientID           *uuid.UUID `json:"client_id,omitempty"`
	ClientPhonePending *string    `json:"client_phone_pending,omitempty"`
	CreatedVia         string     `json:"created_via"` // producer_added | guest_link
	CreatedAt          time.Time  `json:"created_at"`
}

// Message mirrors the messages table.
type Message struct {
	ID       uuid.UUID `json:"id"`
	ChatID   uuid.UUID `json:"chat_id"`
	Type     string    `json:"type"` // text | voice | system
	Text     *string   `json:"text,omitempty"`
	VoiceURL *string   `json:"voice_url,omitempty"`
	AuthorID uuid.UUID `json:"author_id"`
	Ts       time.Time `json:"ts"`
}

// CreateChatInput — producer adds a client by phone (path a, ТЗ 12.7).
type CreateChatInput struct {
	ClientPhone string `json:"client_phone"` // E.164
}

// CreateMessageInput — send a text message.
type CreateMessageInput struct {
	ChatID uuid.UUID `json:"chat_id"`
	Text   string    `json:"text"`
}

type validationError struct{ msg string }

func (e validationError) Error() string { return e.msg }
func errValidation(msg string) error    { return validationError{msg} }

// IsValidationError returns true for user-input errors (→ 400).
func IsValidationError(err error) bool {
	_, ok := err.(validationError)
	return ok
}
