package guest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GET /api/guest/catalog/{producer_token}
// Public — no auth required.
func (h *Handler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("producer_token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "producer_token required")
		return
	}
	catalog, err := h.svc.GetCatalog(r.Context(), token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "producer not found")
			return
		}
		log.Error().Err(err).Msg("guest: GetCatalog failed")
		writeError(w, http.StatusInternalServerError, "internal_server_error")
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

// POST /api/guest/cart
// Body: { "producer_token": "...", "session_id": "optional-uuid", "product_id": "...", "name": "...", "price": 0, "qty": 1 }
func (h *Handler) UpsertCart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProducerToken string    `json:"producer_token"`
		SessionID     string    `json:"session_id"`
		ProductID     uuid.UUID `json:"product_id"`
		Name          string    `json:"name"`
		Price         float64   `json:"price"`
		Qty           float64   `json:"qty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.ProducerToken == "" {
		writeError(w, http.StatusBadRequest, "producer_token required")
		return
	}
	if body.ProductID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "product_id required")
		return
	}

	var sessID *uuid.UUID
	if body.SessionID != "" {
		id, err := uuid.Parse(body.SessionID)
		if err == nil {
			sessID = &id
		}
	}

	item := CartItem{
		ProductID: body.ProductID,
		Name:      body.Name,
		Price:     body.Price,
		Qty:       body.Qty,
	}

	sess, err := h.svc.AddToCart(r.Context(), body.ProducerToken, sessID, item)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "producer not found")
			return
		}
		log.Error().Err(err).Msg("guest: UpsertCart failed")
		writeError(w, http.StatusInternalServerError, "internal_server_error")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// GET /api/guest/cart/{session_id}
func (h *Handler) GetCart(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("session_id")
	sessID, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session_id")
		return
	}
	sess, err := h.svc.GetCart(r.Context(), sessID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found or expired")
			return
		}
		log.Error().Err(err).Msg("guest: GetCart failed")
		writeError(w, http.StatusInternalServerError, "internal_server_error")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("guest: writeJSON error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
