package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"gemstore/internal/httputil"
	"gemstore/internal/models"
	"gemstore/internal/repository"
	"gemstore/internal/service"
)

type ProductHandler struct {
	svc *service.ProductService
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// List handles GET /api/v1/products?category=ring&gender=women
// (Section 5.1: "Product catalogue with filters for category and gender.")
func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := models.ProductFilter{}

	if raw := q.Get("category"); raw != "" {
		c := models.JewelryCategory(raw)
		filter.Category = &c
	}
	if raw := q.Get("gender"); raw != "" {
		g := models.Gender(raw)
		filter.Gender = &g
	}

	products, err := h.svc.ListCatalogue(r.Context(), filter)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	// Always return a JSON array, never null, so frontend code can .map()
	// the response without a null-check.
	if products == nil {
		products = []models.Product{}
	}

	httputil.WriteJSON(w, http.StatusOK, products)
}

// Get handles GET /api/v1/products/{id}
func (h *ProductHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := h.svc.GetProduct(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, product)
}

// writeServiceError maps errors coming out of the service layer to the
// right HTTP status, without ever leaking raw internal/DB error strings
// to the client.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid service.ErrInvalidInput

	switch {
	case errors.As(err, &invalid):
		httputil.WriteError(w, http.StatusBadRequest, invalid.Error())
	case errors.Is(err, repository.ErrNotFound):
		httputil.WriteError(w, http.StatusNotFound, "resource not found")
	default:
		slog.ErrorContext(r.Context(), "unhandled service error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
