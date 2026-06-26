package analytics

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// GET /api/analytics/dashboard
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	producerID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	d, err := h.svc.GetDashboard(r.Context(), producerID)
	if err != nil {
		log.Error().Err(err).Str("producer", producerID.String()).Msg("analytics: dashboard failed")
		writeError(w, http.StatusInternalServerError, "internal_server_error")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("analytics: writeJSON error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
