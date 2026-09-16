package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/models"
)

var ErrNotFound = errors.New("repository: record not found")

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

// List returns active catalogue products matching the given filter.
// Every value from the filter is passed as a bind parameter ($1, $2, ...)
// — the WHERE clause is built by appending fixed SQL fragments, never by
// interpolating user input into the query string.
func (r *ProductRepository) List(ctx context.Context, filter models.ProductFilter) ([]models.Product, error) {
	var (
		conditions []string
		args       []any
	)

	conditions = append(conditions, "is_active = true")

	if filter.Category != nil {
		args = append(args, string(*filter.Category))
		conditions = append(conditions, fmt.Sprintf("category = $%d", len(args)))
	}
	if filter.Gender != nil {
		args = append(args, string(*filter.Gender))
		conditions = append(conditions, fmt.Sprintf("gender = $%d", len(args)))
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit)
	limitPos := len(args)
	args = append(args, offset)
	offsetPos := len(args)

	query := fmt.Sprintf(`
		SELECT id, sku, name, description, category, gender, metal_type,
		       gemstone_type, base_price, stock_quantity, image_url,
		       is_active, created_at, updated_at
		FROM products
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(conditions, " AND "), limitPos, offsetPos)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repository: list products: %w", err)
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(
			&p.ID, &p.SKU, &p.Name, &p.Description, &p.Category, &p.Gender,
			&p.MetalType, &p.GemstoneType, &p.BasePrice, &p.StockQuantity,
			&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: scan product row: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate product rows: %w", err)
	}

	return products, nil
}

// GetByID fetches a single active-or-not product by its UUID.
func (r *ProductRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	const query = `
		SELECT id, sku, name, description, category, gender, metal_type,
		       gemstone_type, base_price, stock_quantity, image_url,
		       is_active, created_at, updated_at
		FROM products
		WHERE id = $1`

	var p models.Product
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.SKU, &p.Name, &p.Description, &p.Category, &p.Gender,
		&p.MetalType, &p.GemstoneType, &p.BasePrice, &p.StockQuantity,
		&p.ImageURL, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get product by id: %w", err)
	}

	return &p, nil
}
