package database

import (
	"context"
	// "fmt"
	// "os"

	// "github.com/jackc/pgx/v5/pgxpool"
)

// NewDBPool creates a new PostgreSQL connection pool.
// DSN (Data Source Name) example: "postgres://user:password@host:port/dbname?sslmode=disable"
// func NewDBPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
// 	config, err := pgxpool.ParseConfig(dsn)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse pgxpool config: %w", err)
// 	}

// 	// You can set pool configurations here, e.g.:
// 	// config.MaxConns = 10
// 	// config.MinConns = 2
// 	// config.MaxConnLifetime = time.Hour
// 	// config.MaxConnIdleTime = time.Minute * 30

// 	pool, err := pgxpool.NewWithConfig(ctx, config)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
// 	}

// 	if err := pool.Ping(ctx); err != nil {
//      pool.Close() // Close the pool if ping fails
// 		return nil, fmt.Errorf("failed to ping database: %w", err)
// 	}

// 	return pool, nil
// }

// Placeholder comment:
// TODO: Implement PostgreSQL connection pool using pgx/v5/pgxpool
// Ensure DSN is loaded from config/env
