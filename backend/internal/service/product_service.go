package service

import (
	"context"

	"github.com/google/uuid"

	"gemstore/internal/models"
	"gemstore/internal/repository"
)

type ProductService struct {
	repo *repository.ProductRepository
}

func NewProductService(repo *repository.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

// ListCatalogue applies default/clamped pagination and passes validated
// filters straight through to the repository. Category/gender values are
// validated here so an invalid filter never reaches the query builder.
func (s *ProductService) ListCatalogue(ctx context.Context, filter models.ProductFilter) ([]models.Product, error) {
	if filter.Category != nil && !filter.Category.Valid() {
		return nil, ErrInvalidInput{Field: "category", Reason: "unrecognized category"}
	}
	if filter.Gender != nil && !filter.Gender.Valid() {
		return nil, ErrInvalidInput{Field: "gender", Reason: "unrecognized gender"}
	}
	return s.repo.List(ctx, filter)
}

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.repo.GetByID(ctx, id)
}
