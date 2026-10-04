package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/models"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// Create stores a new refresh token record. tokenHash must already be
// hashed (SHA-256, hex-encoded) — the repository never sees or stores a
// raw token; see AuthService's generateRefreshToken/hashToken.
func (r *RefreshTokenRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
) error {
	const query = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`
	if _, err := r.pool.Exec(ctx, query, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("repository: create refresh token: %w", err)
	}
	return nil
}

// GetByHash looks up a refresh token by its hash. Returns ErrNotFound
// if no such hash exists — the caller (AuthService.RefreshAccessToken)
// treats that identically to an invalid/expired token, never revealing
// which case it was.
func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	const query = `
		SELECT id, user_id, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1`

	var t models.RefreshToken
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(&t.ID, &t.UserID, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repository: get refresh token by hash: %w", err)
	}
	return &t, nil
}

// Revoke marks a single refresh token as no longer usable — called on
// every successful rotation (the just-used token is revoked as part of
// issuing its replacement) and on logout.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, tokenHash string) error {
	const query = `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`
	if _, err := r.pool.Exec(ctx, query, tokenHash); err != nil {
		return fmt.Errorf("repository: revoke refresh token: %w", err)
	}
	return nil
}

// RevokeAllForUser invalidates every refresh token belonging to userID.
// Called when AuthService.RefreshAccessToken sees an already-revoked
// token presented again — a legitimate client never does this by
// construction (each refresh response sets a brand new token), so reuse
// is treated as a signal of possible theft and every session for that
// user is forced to re-authenticate.
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	const query = `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := r.pool.Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("repository: revoke all refresh tokens for user: %w", err)
	}
	return nil
}
