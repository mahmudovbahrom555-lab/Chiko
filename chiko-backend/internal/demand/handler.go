package demand

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

// GET /api/demand?chat_id=UUID
// Список "что нужно заказать" — сортировка: urgent первыми.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	chatID, err := uuid.Parse(r.URL.Query().Get("chat_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "chat_id query param required")
		return
	}
	items, err := h.svc.List(r.Context(), chatID, callerID)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /api/demand
// Розница добавляет позицию. Поле urgency: urgent | soon | planned.
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var in CreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	it, err := h.svc.Add(r.Context(), in, callerID)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

// PUT /api/demand/{id}
// Любой участник чата может редактировать.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in UpdateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	it, err := h.svc.Update(r.Context(), itemID, callerID, in)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

// DELETE /api/demand/{id}
// Только создатель может удалить.
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Remove(r.Context(), itemID, callerID); err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/demand/suggestions?chat_id=UUID
// Производитель вызывает перед созданием черновика.
// Возвращает каждую непокрытую позицию спроса с до 3 вариантов из каталога.
// Производитель смотрит и явно выбирает что сопоставить.
func (h *Handler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	chatID, err := uuid.Parse(r.URL.Query().Get("chat_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "chat_id query param required")
		return
	}
	suggestions, err := h.svc.GetSuggestions(r.Context(), chatID, callerID)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// POST /api/demand/create-draft
// Производитель отправляет ЯВНЫЕ сопоставления demand_item → product.
// Никакого автоматического fuzzy-assign.
//
// Body:
// {
//   "chat_id": "...",
//   "mappings": [
//     { "demand_item_id": "...", "product_id": "..." },
//     ...
//   ]
// }
func (h *Handler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var body struct {
		ChatID   uuid.UUID `json:"chat_id"`
		Mappings []Mapping `json:"mappings"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ChatID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "chat_id required")
		return
	}
	if len(body.Mappings) == 0 {
		writeError(w, http.StatusBadRequest, "mappings required")
		return
	}

	orderID, err := h.svc.CreateDraftFromMappings(r.Context(), body.ChatID, callerID, body.Mappings)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"order_id": orderID.String()})
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
		log.Error().Err(err).Msg("demand: writeJSON error")
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
	log.Error().Err(err).Msg("demand: internal error")
	writeError(w, http.StatusInternalServerError, "internal_server_error")
}
