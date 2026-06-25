package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/chat"
	"github.com/chiko/backend/internal/middleware"
)

// Handler handles bootstrap and push-token registration (ТЗ раздел 12.7).
type Handler struct {
	db      *pgxpool.Pool
	chatSvc *chat.Service
}

func NewHandler(db *pgxpool.Pool, chatSvc *chat.Service) *Handler {
	return &Handler{db: db, chatSvc: chatSvc}
}

// ── POST /api/auth/bootstrap ─────────────────────────────────────────────────
// Called by Flutter immediately after successful OTP verification.
// Creates producers record if not exists + links any pending chats.

func (h *Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	result, err := h.bootstrapUser(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Str("user", userID.String()).Msg("users: bootstrap failed")
		http.Error(w, `{"error":"bootstrap_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

type bootstrapResult struct {
	ProducerID    uuid.UUID `json:"producer_id"`
	IsNew         bool      `json:"is_new"`
	ChatsLinked   int       `json:"chats_linked"`
}

func (h *Handler) bootstrapUser(ctx context.Context, userID uuid.UUID) (bootstrapResult, error) {
	// 1. Find the user's phone from auth.users.
	var phone string
	_ = h.db.QueryRow(ctx, `SELECT COALESCE(phone,'') FROM auth.users WHERE id=$1`, userID).Scan(&phone)

	// 2. Upsert producer record (idempotent — safe to call on every login).
	var isNew bool
	err := h.db.QueryRow(ctx, `
		INSERT INTO producers (id, name, catalog_currency, guest_token)
		VALUES ($1, '', 'UZS', gen_random_uuid())
		ON CONFLICT (id) DO NOTHING
		RETURNING true
	`, userID).Scan(&isNew)
	if err == pgx.ErrNoRows {
		isNew = false // already existed
	} else if err != nil {
		return bootstrapResult{}, fmt.Errorf("bootstrap upsert producer: %w", err)
	}

	// 3. Create Trial subscription if producer is new.
	if isNew {
		if _, err := h.db.Exec(ctx, `
			INSERT INTO subscriptions (producer_id, plan_id, trial_ends_at, status)
			SELECT $1, id, NOW() + INTERVAL '90 days', 'trial'
			FROM   plans WHERE name = 'Trial' AND active = TRUE
			ON CONFLICT (producer_id) DO NOTHING
		`, userID); err != nil {
			log.Error().Err(err).Str("user", userID.String()).Msg("bootstrap: failed to create trial subscription")
		}
	}

	// 4. Link pending chats where client_phone_pending = this user's phone.
	var chatsLinked int
	if phone != "" {
		tag, err := h.db.Exec(ctx, `
			UPDATE chats
			SET    client_id           = $1,
			       client_phone_pending = NULL
			WHERE  client_phone_pending = $2
			  AND  client_id IS NULL
		`, userID, phone)
		if err != nil {
			log.Error().Err(err).Msg("bootstrap: failed to link pending chats")
		} else {
			chatsLinked = int(tag.RowsAffected())
		}
	}

	return bootstrapResult{
		ProducerID:  userID,
		IsNew:       isNew,
		ChatsLinked: chatsLinked,
	}, nil
}

// ── PUT /api/users/push-token ─────────────────────────────────────────────────
// Flutter calls this on every app start and on onTokenRefresh (ТЗ раздел 12.7).

func (h *Handler) UpdatePushToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		PushToken string `json:"push_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PushToken == "" {
		http.Error(w, `{"error":"push_token required"}`, http.StatusBadRequest)
		return
	}

	if _, err := h.db.Exec(ctx(r), `
		UPDATE producers SET push_token = $2, push_enabled = TRUE
		WHERE id = $1
	`, userID, body.PushToken); err != nil {
		log.Error().Err(err).Str("user", userID.String()).Msg("users: update push token failed")
		http.Error(w, `{"error":"internal_server_error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func ctx(r *http.Request) context.Context { return r.Context() }
