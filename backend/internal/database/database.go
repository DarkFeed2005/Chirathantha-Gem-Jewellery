// Package database owns the pgxpool.Pool lifecycle: opening it with the
// right settings, verifying connectivity at startup, and closing it
// cleanly on shutdown. Nothing else in the codebase should call
// pgxpool.New directly — everything goes through this one entry point so
// pool tuning stays in one place.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gemstore/internal/config"
)

// Connect opens a pgxpool against cfg.DatabaseURL and pings it once to
// fail fast on bad credentials or an unreachable host, rather than
// discovering that on the first request.
func Connect(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: parsing DATABASE_URL: %w", err)
	}

	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolCfg.ConnConfig.ConnectTimeout = cfg.DBConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: creating pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}

	return pool, nil
}

// HealthCheck is used by the /health endpoint. It uses a short, separate
// timeout so a slow DB doesn't hang the health probe indefinitely.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return pool.Ping(ctx)
}
