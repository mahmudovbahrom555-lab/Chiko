package catalog

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/chiko/backend/internal/middleware"
)

// Handler bundles all catalog HTTP handlers.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes wires catalog routes into the given mux.
// All routes expect the user to be authenticated (auth middleware applied by caller).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog/categories",               h.ListCategories)
	mux.HandleFunc("POST /api/catalog/categories",              h.CreateCategory)
	mux.HandleFunc("GET /api/catalog/products",                 h.ListProducts)
	mux.HandleFunc("POST /api/catalog/products",                h.CreateProduct)
	mux.HandleFunc("PUT /api/catalog/products/{id}",            h.UpdateProduct)
	mux.HandleFunc("DELETE /api/catalog/products/{id}",         h.DeleteProduct)
	mux.HandleFunc("GET /api/catalog/template",                 h.DownloadTemplate)
	mux.HandleFunc("POST /api/catalog/import",                  h.ImportProducts)
	mux.HandleFunc("GET /api/catalog/export",                   h.ExportCatalog)
	mux.HandleFunc("PUT /api/producers/{id}/currency",          h.SetCurrency)
	mux.HandleFunc("GET /api/catalog/currencies",               h.SearchCurrencies)
}

// ── Categories ────────────────────────────────────────────────────────────────

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	cats, err := h.svc.ListCategories(r.Context(), producerID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cats)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	cat, err := h.svc.CreateCategory(r.Context(), producerID, body.Name)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cat)
}

// ── Products ──────────────────────────────────────────────────────────────────

func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	callerID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Optional producer_id param — clients use this to browse a specific producer's catalog.
	// Without it, defaults to the caller's own products (producer self-view).
	// RLS enforces that the caller has access (either IS the producer, or has a chat with them).
	producerID := callerID
	if raw := r.URL.Query().Get("producer_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid producer_id")
			return
		}
		producerID = id
	}

	q := r.URL.Query()
	p := SearchParams{
		Query:  q.Get("q"),
		Limit:  intParam(q.Get("limit"), 50),
		Offset: intParam(q.Get("offset"), 0),
	}
	if raw := q.Get("category_id"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			p.CategoryID = &id
		}
	}

	products, err := h.svc.ListProducts(r.Context(), producerID, p)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	var in CreateProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr, err := h.svc.CreateProduct(r.Context(), producerID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, pr)
}

func (h *Handler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	productID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var in UpdateProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr, err := h.svc.UpdateProduct(r.Context(), producerID, productID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	productID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProduct(r.Context(), producerID, productID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Currency ──────────────────────────────────────────────────────────────────

func (h *Handler) SetCurrency(w http.ResponseWriter, r *http.Request) {
	producerID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	callerID, _ := middleware.UserIDFromCtx(r.Context())
	if callerID != producerID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := h.svc.SetCurrency(r.Context(), producerID, body.Code); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SearchCurrencies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	result, err := h.svc.SearchCurrencies(r.Context(), q)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ── Excel ─────────────────────────────────────────────────────────────────────

func (h *Handler) DownloadTemplate(w http.ResponseWriter, _ *http.Request) {
	data, err := ExportTemplate()
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="catalog_template.xlsx"`)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *Handler) ImportProducts(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field missing")
		return
	}
	defer file.Close()

	// preview=true → return first 10 rows without saving (ТЗ раздел 9.3)
	previewOnly := r.FormValue("preview") == "true"

	preview, all, warnings, err := ParseImportFile(file, 10)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if previewOnly {
		writeJSON(w, http.StatusOK, ImportPreview{
			Rows:     preview,
			Total:    len(all),
			Warnings: warnings,
		})
		return
	}

	created, importWarnings, err := h.svc.ImportProducts(r.Context(), producerID, all)
	if err != nil {
		internalError(w, err)
		return
	}
	warnings = append(warnings, importWarnings...)
	writeJSON(w, http.StatusOK, map[string]any{
		"created":  created,
		"warnings": warnings,
	})
}

func (h *Handler) ExportCatalog(w http.ResponseWriter, r *http.Request) {
	producerID := mustProducerID(w, r)
	if producerID == uuid.Nil {
		return
	}
	products, err := h.svc.ListProducts(r.Context(), producerID, SearchParams{Limit: 10000})
	if err != nil {
		internalError(w, err)
		return
	}
	cats, err := h.svc.ListCategories(r.Context(), producerID)
	if err != nil {
		internalError(w, err)
		return
	}
	data, err := ExportCatalog(products, cats)
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="catalog.xlsx"`)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ── shared helpers ────────────────────────────────────────────────────────────

func mustProducerID(w http.ResponseWriter, r *http.Request) uuid.UUID {
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
		log.Error().Err(err).Msg("catalog: writeJSON encode error")
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("catalog: internal error")
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
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}
