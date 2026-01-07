package db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ssuji15/wolf/internal/config"
)

type DB struct {
	Pool *pgxpool.Pool
}

var (
	d         *DB
	once      sync.Once
	initError error
)

func New(ctx context.Context) (*DB, error) {

	once.Do(func() {
		c, err := config.GetPostgresConfig()
		if err != nil {
			initError = err
			return
		}
		cfg, err := pgxpool.ParseConfig(c.URL)
		if err != nil {
			initError = fmt.Errorf("failed to parse postgres config: %w", err)
			return
		}

		cfg.MaxConns = 20
		cfg.MinConns = 10
		cfg.MaxConnLifetime = time.Hour
		cfg.HealthCheckPeriod = 30 * time.Second
		cfg.MaxConnIdleTime = 15 * time.Minute

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			initError = fmt.Errorf("failed to connect to postgres: %w", err)
			return
		}

		// Test connection
		if err := pool.Ping(ctx); err != nil {
			initError = fmt.Errorf("failed to ping postgres: %w", err)
			return
		}

		d = &DB{Pool: pool}
	})

	return d, initError
}

func Close() {
	d.Pool.Close()
}
