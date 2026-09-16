package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	appmiddleware "gemstore/internal/middleware"

	"gemstore/internal/httputil"
	"gemstore/internal/models"
	"gemstore/internal/service"
)

type AdminHandler struct {
	svc *service.ApprovalService
}

func NewAdminHandler(svc *service.ApprovalService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// ListPending handles GET /api/v1/admin/orders/pending
// (Section 5.2: "Admin dashboard summarizing pending custom orders").
func (h *AdminHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	orders, err := h.svc.ListPendingOrders(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	// Same rationale as ProductHandler.List: never return a bare JSON
	// null for an empty result set.
	if orders == nil {
		orders = []models.PendingCustomOrder{}
	}

	httputil.WriteJSON(w, http.StatusOK, orders)
}

// Approve handles POST /api/v1/admin/orders/{id}/approve
// (Section 8, steps 4-5: owner confirms; customer is notified automatically).
func (h *AdminHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.svc.ApproveCustomOrder, false)
}

// Decline handles POST /api/v1/admin/orders/{id}/decline. Unlike
// Approve, the service layer requires non-empty notes here — that's
// enforced in ApprovalService.DeclineCustomOrder, not repeated here, so
// there's exactly one place the "reason required" rule lives.
func (h *AdminHandler) Decline(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.svc.DeclineCustomOrder, true)
}

// decideFunc is the shape ApproveCustomOrder and DeclineCustomOrder both
// have — extracting it lets decide() below handle request parsing, auth
// context, and error mapping once for both endpoints instead of twice.
type decideFunc func(ctx context.Context, orderID, adminID uuid.UUID, notes *string) (*models.Order, error)

func (h *AdminHandler) decide(w http.ResponseWriter, r *http.Request, fn decideFunc, reasonRequired bool) {
	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	// Set by middleware.Authenticate; RequireRole(RoleAdmin) on this
	// route group guarantees the role is admin by the time we get here.
	adminID, ok := appmiddleware.UserIDFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	var req models.DecisionRequest
	// An empty body is fine for Approve (notes are optional there); only
	// reject genuinely malformed JSON.
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "malformed JSON body")
			return
		}
	}
	if reasonRequired && (req.Notes == nil || *req.Notes == "") {
		httputil.WriteError(w, http.StatusBadRequest, "notes is required when declining an order")
		return
	}

	order, err := fn(r.Context(), orderID, adminID, req.Notes)
	if err != nil {
		writeApprovalError(w, r, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, order)
}

// writeApprovalError extends writeServiceError with the one error type
// specific to the approval workflow: attempting to decide an order that
// isn't awaiting review anymore (already decided, or not a custom
// order). That's a conflict with the resource's current state, so it
// maps to 409 rather than 400 (bad input) or 404 (doesn't exist) — the
// order does exist, the request is well-formed, it's just too late.
func writeApprovalError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrOrderNotPending) {
		httputil.WriteError(w, http.StatusConflict, "order is not awaiting review (already decided, or not a custom order)")
		return
	}
	writeServiceError(w, r, err)
}
