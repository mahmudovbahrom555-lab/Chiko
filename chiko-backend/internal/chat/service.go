package chat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/ws"
)

// Service handles chat creation and messaging (ТЗ раздел 12.7, 9.1).
type Service struct {
	db         *pgxpool.Pool
	hub        *ws.Hub
	storageURL string // Supabase Storage base URL
	storageKey string // service_role key for uploads
}

func NewService(db *pgxpool.Pool, hub *ws.Hub, storageURL, storageKey string) *Service {
	return &Service{db: db, hub: hub, storageURL: storageURL, storageKey: storageKey}
}

// ──────────────────────────── CHATS ──────────────────────────────────────────

// ListChats returns all chats for the authenticated user.
func (s *Service) ListChats(ctx context.Context, userID uuid.UUID) ([]Chat, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, producer_id, client_id, client_phone_pending, created_via, created_at
		FROM   chats
		WHERE  producer_id = $1 OR client_id = $1
		ORDER  BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("chat.ListChats: %w", err)
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.ProducerID, &c.ClientID,
			&c.ClientPhonePending, &c.CreatedVia, &c.CreatedAt); err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// CreateChat path (a): producer adds client by phone.
// If client not yet registered → store client_phone_pending.
func (s *Service) CreateChat(ctx context.Context, producerID uuid.UUID, in CreateChatInput) (Chat, error) {
	if in.ClientPhone == "" {
		return Chat{}, errValidation("client_phone is required")
	}

	// Check for duplicate chat for this producer+phone.
	var existingID uuid.UUID
	err := s.db.QueryRow(ctx, `
		SELECT c.id FROM chats c
		WHERE  c.producer_id = $1
		  AND (
		       c.client_phone_pending = $2
		    OR EXISTS (
		           SELECT 1 FROM auth.users u
		           WHERE  u.id = c.client_id AND u.phone = $2
		       )
		  )
		LIMIT 1
	`, producerID, in.ClientPhone).Scan(&existingID)
	if err == nil {
		return s.getChat(ctx, existingID)
	}
	if err != pgx.ErrNoRows {
		return Chat{}, fmt.Errorf("chat.CreateChat duplicate check: %w", err)
	}

	// Find auth user by phone (service_role connection sees auth.users).
	var clientID *uuid.UUID
	var uid uuid.UUID
	if lookupErr := s.db.QueryRow(ctx,
		`SELECT id FROM auth.users WHERE phone = $1 LIMIT 1`, in.ClientPhone,
	).Scan(&uid); lookupErr == nil {
		clientID = &uid
	} else if lookupErr != pgx.ErrNoRows {
		return Chat{}, fmt.Errorf("chat.CreateChat user lookup: %w", lookupErr)
	}

	var c Chat
	if clientID != nil {
		err = s.db.QueryRow(ctx, `
			INSERT INTO chats (producer_id, client_id, created_via)
			VALUES ($1, $2, 'producer_added')
			RETURNING id, producer_id, client_id, client_phone_pending, created_via, created_at
		`, producerID, *clientID).Scan(
			&c.ID, &c.ProducerID, &c.ClientID,
			&c.ClientPhonePending, &c.CreatedVia, &c.CreatedAt)
	} else {
		err = s.db.QueryRow(ctx, `
			INSERT INTO chats (producer_id, client_phone_pending, created_via)
			VALUES ($1, $2, 'producer_added')
			RETURNING id, producer_id, client_id, client_phone_pending, created_via, created_at
		`, producerID, in.ClientPhone).Scan(
			&c.ID, &c.ProducerID, &c.ClientID,
			&c.ClientPhonePending, &c.CreatedVia, &c.CreatedAt)
	}
	if err != nil {
		return Chat{}, fmt.Errorf("chat.CreateChat insert: %w", err)
	}
	return c, nil
}

// LinkPendingChats resolves all chats with client_phone_pending = phone
// and assigns them to userID. Called during bootstrap on first login.
func (s *Service) LinkPendingChats(ctx context.Context, userID uuid.UUID, phone string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE chats
		SET    client_id           = $1,
		       client_phone_pending = NULL
		WHERE  client_phone_pending = $2
		  AND  client_id IS NULL
	`, userID, phone)
	if err != nil {
		return fmt.Errorf("chat.LinkPendingChats: %w", err)
	}
	if tag.RowsAffected() > 0 {
		log.Info().
			Str("user", userID.String()).
			Str("phone", phone).
			Int64("linked", tag.RowsAffected()).
			Msg("chat: linked pending chats on first login")
	}
	return nil
}

func (s *Service) getChat(ctx context.Context, id uuid.UUID) (Chat, error) {
	var c Chat
	err := s.db.QueryRow(ctx, `
		SELECT id, producer_id, client_id, client_phone_pending, created_via, created_at
		FROM   chats WHERE id = $1
	`, id).Scan(&c.ID, &c.ProducerID, &c.ClientID,
		&c.ClientPhonePending, &c.CreatedVia, &c.CreatedAt)
	if err != nil {
		return Chat{}, fmt.Errorf("chat.getChat: %w", err)
	}
	return c, nil
}

// ──────────────────────────── MESSAGES ───────────────────────────────────────

// isChatParticipant returns error if callerID is not a member of the chat.
// Fail-closed: DB errors are logged and treated as "not a participant".
func (s *Service) isChatParticipant(ctx context.Context, chatID, callerID uuid.UUID) error {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM chats WHERE id=$1 AND (producer_id=$2 OR client_id=$2))
	`, chatID, callerID).Scan(&exists)
	if err != nil {
		log.Warn().Err(err).Str("chat", chatID.String()).Str("caller", callerID.String()).
			Msg("chat: isChatParticipant DB error")
		return errValidation("not a participant of this chat")
	}
	if !exists {
		return errValidation("not a participant of this chat")
	}
	return nil
}

// ListMessages returns messages for a chat ordered by time.
func (s *Service) ListMessages(ctx context.Context, callerID, chatID uuid.UUID, limit, offset int) ([]Message, error) {
	if err := s.isChatParticipant(ctx, chatID, callerID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, chat_id, type, text, voice_url, author_id, ts
		FROM   messages
		WHERE  chat_id = $1
		ORDER  BY ts
		LIMIT  $2 OFFSET $3
	`, chatID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("chat.ListMessages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChatID, &m.Type, &m.Text,
			&m.VoiceURL, &m.AuthorID, &m.Ts); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// SendText creates a text message and broadcasts message.new.
func (s *Service) SendText(ctx context.Context, callerID uuid.UUID, in CreateMessageInput) (Message, error) {
	if strings.TrimSpace(in.Text) == "" {
		return Message{}, errValidation("text is required")
	}
	if in.ChatID == uuid.Nil {
		return Message{}, errValidation("chat_id is required")
	}
	if err := s.isChatParticipant(ctx, in.ChatID, callerID); err != nil {
		return Message{}, err
	}

	var m Message
	err := s.db.QueryRow(ctx, `
		INSERT INTO messages (chat_id, type, text, author_id)
		VALUES ($1, 'text', $2, $3)
		RETURNING id, chat_id, type, text, voice_url, author_id, ts
	`, in.ChatID, in.Text, callerID).Scan(
		&m.ID, &m.ChatID, &m.Type, &m.Text, &m.VoiceURL, &m.AuthorID, &m.Ts)
	if err != nil {
		return Message{}, fmt.Errorf("chat.SendText: %w", err)
	}

	s.broadcastMessage(m)
	return m, nil
}

// SendVoice uploads audio to Supabase Storage, creates a voice message.
// Supported formats: opus, ogg, m4a (ТЗ раздел 9.1).
func (s *Service) SendVoice(ctx context.Context, callerID, chatID uuid.UUID, audio io.Reader, ext string) (Message, error) {
	if err := s.isChatParticipant(ctx, chatID, callerID); err != nil {
		return Message{}, err
	}
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "opus", "ogg", "m4a", "mp4", "webm":
		// accepted
	case "":
		ext = "ogg"
	default:
		return Message{}, errValidation("unsupported audio format — use opus or m4a")
	}

	msgID := uuid.New()
	objectPath := fmt.Sprintf("voice-messages/%s/%s.%s", chatID, msgID, ext)
	voiceURL, err := s.uploadToStorage(ctx, objectPath, audio, "audio/"+ext)
	if err != nil {
		return Message{}, fmt.Errorf("chat.SendVoice upload: %w", err)
	}

	var m Message
	err = s.db.QueryRow(ctx, `
		INSERT INTO messages (id, chat_id, type, voice_url, author_id)
		VALUES ($1, $2, 'voice', $3, $4)
		RETURNING id, chat_id, type, text, voice_url, author_id, ts
	`, msgID, chatID, voiceURL, callerID).Scan(
		&m.ID, &m.ChatID, &m.Type, &m.Text, &m.VoiceURL, &m.AuthorID, &m.Ts)
	if err != nil {
		return Message{}, fmt.Errorf("chat.SendVoice insert: %w", err)
	}

	s.broadcastMessage(m)
	return m, nil
}

func (s *Service) broadcastMessage(m Message) {
	payload := ws.MessageNewPayload{
		MessageID: m.ID,
		ChatID:    m.ChatID,
		AuthorID:  m.AuthorID,
		Type:      m.Type,
	}
	if m.Text != nil {
		payload.Text = *m.Text
	}
	if m.VoiceURL != nil {
		payload.VoiceURL = *m.VoiceURL
	}
	ev, err := ws.NewEvent(ws.EventMessageNew, payload)
	if err != nil {
		return
	}
	encoded, _ := ev.Encode()
	// Don't exclude sender — both sides need to see the message.
	s.hub.Broadcast(ws.BroadcastMsg{ChatID: m.ChatID, Data: encoded})
}

// uploadToStorage uploads bytes to Supabase Storage and returns the public URL.
func (s *Service) uploadToStorage(ctx context.Context, path string, r io.Reader, contentType string) (string, error) {
	if s.storageURL == "" {
		return "", fmt.Errorf("SUPABASE_STORAGE_URL not configured")
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read audio: %w", err)
	}

	const bucket = "chiko-media"
	uploadURL := fmt.Sprintf("%s/object/%s/%s", s.storageURL, bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.storageKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("storage upload HTTP %d", resp.StatusCode)
	}

	// Public URL for Supabase Storage.
	_ = filepath.Base // ensure filepath is used
	return fmt.Sprintf("%s/object/public/%s/%s", s.storageURL, bucket, path), nil
}
