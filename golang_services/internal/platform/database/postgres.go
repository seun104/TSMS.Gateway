package database

import (
	"context"
	"fmt"
	// "os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDBPool creates a new PostgreSQL connection pool.
// DSN (Data Source Name) example: "postgres://user:password@host:port/dbname?sslmode=disable"
func NewDBPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgxpool config: %w", err)
	}

	// You can set pool configurations here, e.g.:
	config.MaxConns = 10 // Example: Set max connections
	config.MinConns = 2  // Example: Set min connections
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30
	config.HealthCheckPeriod = time.Minute // Periodically check health of connections

	pool, err := pgxpool.NewWithConfig(ctx, config) // Use NewWithConfig
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
        pool.Close() // Close the pool if ping fails
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}

// Placeholder comment:
// TODO: Implement PostgreSQL connection pool using pgx/v5/pgxpool - DONE
// Ensure DSN is loaded from config/env - Handled by main.go passing DSN to this func
