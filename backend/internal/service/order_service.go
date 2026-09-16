package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gemstore/internal/models"
	"gemstore/internal/repository"
)

type OrderService struct {
	repo *repository.OrderRepository
}

func NewOrderService(repo *repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// SubmitCustomOrder validates the customer's customization + order request
// (Section 8, step 3: "system marks it Pending Owner Review") and, if
// valid, delegates to the repository's transactional insert.
//
// Validation lives here rather than in the handler so the same rules
// apply no matter what ever calls this service (HTTP today; a future
// gRPC or admin-CLI entry point tomorrow).
func (s *OrderService) SubmitCustomOrder(
	ctx context.Context,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.Order, error) {
	if err := validateCustomOrderRequest(req); err != nil {
		return nil, err
	}
	return s.repo.CreateCustomOrder(ctx, userID, req)
}

func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	return s.repo.GetByID(ctx, id)
}

func validateCustomOrderRequest(req models.CustomOrderRequest) error {
	if !req.Category.Valid() {
		return ErrInvalidInput{Field: "category", Reason: "must be one of ring, bracelet, necklace"}
	}
	if !req.Gender.Valid() {
		return ErrInvalidInput{Field: "gender", Reason: "must be one of men, women, unisex"}
	}
	if strings.TrimSpace(req.MetalType) == "" {
		return ErrInvalidInput{Field: "metal_type", Reason: "is required"}
	}
	if req.Quantity <= 0 {
		return ErrInvalidInput{Field: "quantity", Reason: "must be at least 1"}
	}
	if req.EstimatedPrice < 0 {
		return ErrInvalidInput{Field: "estimated_price", Reason: "cannot be negative"}
	}
	if req.EngravingText != nil && len(*req.EngravingText) > 100 {
		return ErrInvalidInput{Field: "engraving_text", Reason: "must be 100 characters or fewer"}
	}
	return nil
}
