package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssuji15/wolf/internal/config"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, c config.Config) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(c.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
	d.Pool.Close()
}
