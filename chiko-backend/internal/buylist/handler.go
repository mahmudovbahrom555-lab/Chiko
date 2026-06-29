package buylist

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// POST /api/buy-list
// Публичный (без auth): магазин создаёт список покупок.
// Body: { "producer_phone": "+998901234567", "guest_phone": "...", "lines": [...] }
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	bl, err := h.svc.Create(r.Context(), in)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, bl)
}

// GET /api/buy-list/{token}
// Публичный: просмотр Buy List по guest_token (магазин и поставщик).
func (h *Handler) GetByToken(w http.ResponseWriter, r *http.Request) {
	token, err := uuid.Parse(r.PathValue("token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	bl, err := h.svc.GetByToken(r.Context(), token)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bl)
}

// GET /api/buy-list/{id}/suggestions
// Auth (producer): получить строки с вариантами для сопоставления.
func (h *Handler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	producerID := mustCallerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	suggestions, err := h.svc.GetSuggestions(r.Context(), orderID, producerID)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// POST /api/buy-list/{id}/map
// Auth (producer): сопоставить строки с товарами из каталога.
// Body: { "mappings": [{ "line_id": "...", "product_id": "..." }] }
func (h *Handler) MapLines(w http.ResponseWriter, r *http.Request) {
	producerID := mustCallerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Mappings []MapInput `json:"mappings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Mappings) == 0 {
		writeError(w, http.StatusBadRequest, "mappings required")
		return
	}
	_, err = h.svc.MapLines(r.Context(), orderID, producerID, body.Mappings)
	if err != nil {
		handleErr(w, err)
		return
	}
	bl, err := h.svc.GetByOrderID(r.Context(), orderID)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bl)
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
		log.Error().Err(err).Msg("buylist: writeJSON error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleErr(w http.ResponseWriter, err error) {
	if IsValidationError(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Error().Err(err).Msg("buylist: internal error")
	writeError(w, http.StatusInternalServerError, "internal_server_error")
}
