package order

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
	"github.com/chiko/backend/internal/push"
)

// Handler bundles all order HTTP handlers.
type Handler struct {
	svc      *Service
	pushSvc  *push.Service // optional — nil in tests
}

func NewHandler(svc *Service, pushSvc *push.Service) *Handler {
	return &Handler{svc: svc, pushSvc: pushSvc}
}

// ── POST /api/orders ──────────────────────────────────────────────────────────

func (h *Handler) CreateDraft(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var body struct {
		ChatID uuid.UUID `json:"chat_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ChatID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "chat_id is required")
		return
	}
	o, err := h.svc.CreateDraft(r.Context(), body.ChatID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// ── GET /api/orders?chat_id=... ───────────────────────────────────────────────

func (h *Handler) ListByChat(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	raw := r.URL.Query().Get("chat_id")
	chatID, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "chat_id query param required")
		return
	}
	orders, err := h.svc.ListByChat(r.Context(), chatID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// ── GET /api/orders/{id} ──────────────────────────────────────────────────────

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	orderID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	o, err := h.svc.GetOrder(r.Context(), orderID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// ── PUT /api/orders/{id}/items ────────────────────────────────────────────────

func (h *Handler) UpsertItem(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	orderID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var in UpdateItemInput
	if !decodeJSON(w, r, &in) {
		return
	}
	item, err := h.svc.UpsertItem(r.Context(), orderID, callerID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// ── DELETE /api/orders/{id}/items/{item_id} ───────────────────────────────────

func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	orderID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	itemID, ok := pathUUID(w, r, "item_id")
	if !ok {
		return
	}
	if err := h.svc.RemoveItem(r.Context(), orderID, itemID, callerID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /api/orders/{id}/confirm ────────────────────────────────────────────

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	orderID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}

	o, err := h.svc.Confirm(r.Context(), orderID, callerID)
	if err != nil {
		if IsDailyLimitError(err) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		handleServiceError(w, err)
		return
	}

	// Push to the OTHER side (ТЗ раздел 5, шаг 4.1).
	// order.confirmed: if Client confirmed → push Producer; if Producer → push Client.
	if h.pushSvc != nil {
		h.pushSvc.SendToOpposite(r.Context(), o.ChatID, callerID, push.Payload{
			Type:  push.EventOrderConfirmed,
			Title: "Заказ подтверждён",
			Body:  "Новый подтверждённый заказ",
			Data:  map[string]any{"order_id": o.ID.String(), "total": o.Total},
		})
	}

	writeJSON(w, http.StatusOK, o)
}

// ── POST /api/orders/{id}/repeat ─────────────────────────────────────────────

func (h *Handler) Repeat(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}

	// The :id here is the chat_id from which to repeat the last confirmed order.
	// Alternative: POST /api/chats/{chat_id}/orders/repeat — but we keep URL flat.
	var body struct {
		ChatID uuid.UUID `json:"chat_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ChatID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "chat_id is required")
		return
	}

	result, err := h.svc.Repeat(r.Context(), body.ChatID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
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

func pathUUID(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	raw := r.PathValue(key)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+key)
		return uuid.Nil, false
	}
	return id, true
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
		log.Error().Err(err).Msg("order: writeJSON encode error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("order: internal error")
	writeError(w, http.StatusInternalServerError, "internal_server_error")
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case IsValidationError(err):
		writeError(w, http.StatusBadRequest, err.Error())
	case IsDailyLimitError(err):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		internalError(w, err)
	}
}
