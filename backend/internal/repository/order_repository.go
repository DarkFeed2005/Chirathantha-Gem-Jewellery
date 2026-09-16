package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/models"
)

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// CreateCustomOrder implements Section 8, steps 2-3 of the proposal:
// it persists the customer's customization spec, creates the parent order
// in 'pending_review' status, and links them with an order_item — all in
// a single transaction, so a crash midway never leaves an order without
// its customization or vice versa.
func (r *OrderRepository) CreateCustomOrder(
	ctx context.Context,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("repository: begin tx: %w", err)
	}
	// Rollback is a no-op once Commit has succeeded; this just guarantees
	// we never leave a transaction open on an early return.
	defer func() { _ = tx.Rollback(ctx) }()

	customizationID, err := insertCustomization(ctx, tx, userID, req)
	if err != nil {
		return nil, err
	}

	order, err := insertOrder(ctx, tx, userID, req)
	if err != nil {
		return nil, err
	}

	item, err := insertOrderItem(ctx, tx, order.ID, customizationID, req)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("repository: commit custom order tx: %w", err)
	}

	order.UserID = userID
	order.Items = []models.OrderItem{*item}
	return order, nil
}

func insertCustomization(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (uuid.UUID, error) {
	const query = `
		INSERT INTO customizations
			(user_id, category, gender, metal_type, gemstone_type,
			 size, engraving_text, preview_image_url, estimated_price)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	var id uuid.UUID
	err := tx.QueryRow(ctx, query,
		userID, req.Category, req.Gender, req.MetalType, req.GemstoneType,
		req.Size, req.EngravingText, req.PreviewImageURL, req.EstimatedPrice,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("repository: insert customization: %w", err)
	}
	return id, nil
}

func insertOrder(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.Order, error) {
	const query = `
		INSERT INTO orders (user_id, order_type, status, shipping_address, total_amount)
		VALUES ($1, 'custom', 'pending_review', $2, $3)
		RETURNING id, order_type, status, total_amount, created_at, updated_at`

	totalAmount := req.EstimatedPrice * float64(req.Quantity)

	var order models.Order
	err := tx.QueryRow(ctx, query, userID, req.ShippingAddress, totalAmount).Scan(
		&order.ID, &order.OrderType, &order.Status, &order.TotalAmount,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("repository: insert order: %w", err)
	}
	return &order, nil
}

func insertOrderItem(
	ctx context.Context,
	tx pgx.Tx,
	orderID, customizationID uuid.UUID,
	req models.CustomOrderRequest,
) (*models.OrderItem, error) {
	const query = `
		INSERT INTO order_items (order_id, customization_id, quantity, unit_price)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	item := &models.OrderItem{
		OrderID:         orderID,
		CustomizationID: &customizationID,
		Quantity:        req.Quantity,
		UnitPrice:       req.EstimatedPrice,
	}
	err := tx.QueryRow(ctx, query, orderID, customizationID, req.Quantity, req.EstimatedPrice).
		Scan(&item.ID, &item.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("repository: insert order item: %w", err)
	}
	return item, nil
}

// GetByID fetches an order together with its line items — used to return
// the freshly created resource from the handler (201 + body) and, later,
// by the admin review screen.
func (r *OrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	const orderQuery = `
		SELECT id, user_id, order_type, status, shipping_address, total_amount,
		       created_at, updated_at
		FROM orders
		WHERE id = $1`

	var order models.Order
	err := r.pool.QueryRow(ctx, orderQuery, id).Scan(
		&order.ID, &order.UserID, &order.OrderType, &order.Status,
		&order.ShippingAddress, &order.TotalAmount, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get order by id: %w", err)
	}

	const itemsQuery = `
		SELECT id, order_id, product_id, customization_id, quantity, unit_price, created_at
		FROM order_items
		WHERE order_id = $1
		ORDER BY created_at`

	rows, err := r.pool.Query(ctx, itemsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("repository: list order items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item models.OrderItem
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.ProductID, &item.CustomizationID,
			&item.Quantity, &item.UnitPrice, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("repository: scan order item: %w", err)
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: iterate order items: %w", err)
	}

	return &order, nil
}
