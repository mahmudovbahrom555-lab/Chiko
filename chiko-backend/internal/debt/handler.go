package debt

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
	"github.com/chiko/backend/internal/push"
)

// Handler bundles all debt HTTP handlers.
type Handler struct {
	svc     *Service
	pushSvc *push.Service // optional — nil in tests
}

func NewHandler(svc *Service, pushSvc *push.Service) *Handler {
	return &Handler{svc: svc, pushSvc: pushSvc}
}

// ── GET /api/debt/balance/:chat_id ───────────────────────────────────────────

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathUUID(w, r, "chat_id")
	if !ok {
		return
	}
	b, err := h.svc.GetBalance(r.Context(), chatID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// ── GET /api/debt/history/:chat_id ───────────────────────────────────────────

func (h *Handler) ListHistory(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathUUID(w, r, "chat_id")
	if !ok {
		return
	}
	txs, err := h.svc.ListHistory(r.Context(), chatID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, txs)
}

// ── POST /api/debt/delivery ───────────────────────────────────────────────────

func (h *Handler) CreateDelivery(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var in CreateDeliveryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := h.svc.CreateDelivery(r.Context(), in, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// ── POST /api/debt/payment ────────────────────────────────────────────────────

func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var in CreatePaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := h.svc.CreatePayment(r.Context(), in, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// ── POST /api/debt/transactions/:id/confirm ───────────────────────────────────

func (h *Handler) ConfirmTx(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	txID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.Confirm(r.Context(), txID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ── POST /api/debt/transactions/:id/dispute ───────────────────────────────────

func (h *Handler) DisputeTx(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	txID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.Dispute(r.Context(), txID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ── POST /api/debt/returns ────────────────────────────────────────────────────

func (h *Handler) CreateReturnRequest(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	var in CreateReturnRequestInput
	if !decodeJSON(w, r, &in) {
		return
	}
	rr, err := h.svc.CreateReturnRequest(r.Context(), in, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Push the OTHER side: client filed return → push producer, and vice versa.
	if h.pushSvc != nil {
		h.pushSvc.SendToOpposite(r.Context(), rr.ChatID, callerID, push.Payload{
			Type:  push.EventReturnRequested,
			Title: "Запрос на возврат",
			Body:  "Поступил новый запрос на возврат товара",
			Data:  map[string]any{"return_id": rr.ID.String()},
		})
	}

	writeJSON(w, http.StatusCreated, rr)
}

// ── POST /api/debt/returns/:id/correct ───────────────────────────────────────

func (h *Handler) CreateReturnCorrection(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	returnReqID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Amount  float64 `json:"amount"`
		Comment string  `json:"comment"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	in := CreateReturnCorrectionInput{
		ReturnRequestID: returnReqID,
		Amount:          body.Amount,
		Comment:         body.Comment,
	}
	t, err := h.svc.CreateReturnCorrection(r.Context(), in, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// ── POST /api/debt/transactions/:id/dispute-correction ───────────────────────

func (h *Handler) DisputeCorrection(w http.ResponseWriter, r *http.Request) {
	callerID := mustCallerID(w, r)
	if callerID == uuid.Nil {
		return
	}
	txID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.DisputeCorrection(r.Context(), txID, callerID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
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
		log.Error().Err(err).Msg("debt: writeJSON error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("debt: internal error")
	writeError(w, http.StatusInternalServerError, "internal_server_error")
}

func handleServiceError(w http.ResponseWriter, err error) {
	if IsValidationError(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	internalError(w, err)
}
