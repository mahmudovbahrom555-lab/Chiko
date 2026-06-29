package chat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
)

// Handler bundles all chat HTTP handlers.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ── GET /api/chats ────────────────────────────────────────────────────────────

func (h *Handler) ListChats(w http.ResponseWriter, r *http.Request) {
	userID := mustCallerID(w, r)
	if userID == uuid.Nil {
		return
	}
	chats, err := h.svc.ListChats(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, chats)
}

// ── POST /api/chats ───────────────────────────────────────────────────────────

func (h *Handler) CreateChat(w http.ResponseWriter, r *http.Request) {
	producerID := mustCallerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	var in CreateChatInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.CreateChat(r.Context(), producerID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ── GET /api/messages?chat_id=...&limit=...&offset=... ────────────────────────

func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	q := r.URL.Query()
	chatID, err := uuid.Parse(q.Get("chat_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "chat_id query param required")
		return
	}
	msgs, err := h.svc.ListMessages(r.Context(), callerID, chatID, intParam(q.Get("limit"), 50), intParam(q.Get("offset"), 0))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// ── POST /api/messages ────────────────────────────────────────────────────────

func (h *Handler) SendText(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var in CreateMessageInput
	if !decodeJSON(w, r, &in) {
		return
	}
	m, err := h.svc.SendText(r.Context(), callerID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// ── POST /api/messages/voice ──────────────────────────────────────────────────

func (h *Handler) SendVoice(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	rawChatID := r.FormValue("chat_id")
	chatID, err := uuid.Parse(rawChatID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "chat_id required")
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "audio file required")
		return
	}
	defer file.Close()

	// Derive extension from filename safely.
	// strings.LastIndex returns -1 if no dot found → panic without bounds check.
	var ext string
	if idx := strings.LastIndex(header.Filename, "."); idx >= 0 {
		ext = strings.ToLower(header.Filename[idx+1:])
	}

	m, err := h.svc.SendVoice(r.Context(), callerID, chatID, file, ext)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// ── shared helpers ────────────────────────────────────────────────────────────

func mustCallerID(w http.ResponseWriter, r *http.Request) uuid.UUID {
	id, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil
	}
	return id
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("chat: writeJSON error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("chat: internal error")
	writeError(w, http.StatusInternalServerError, "internal_server_error")
}

func handleServiceError(w http.ResponseWriter, err error) {
	if IsValidationError(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	internalError(w, err)
}

func intParam(s string, def int) int {
	var v int
	if err := json.Unmarshal([]byte(s), &v); err == nil && v > 0 {
		return v
	}
	return def
}
