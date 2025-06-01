package repository

import (
	"context"
	"github.com/aradsms/golang_services/internal/billing_service/domain"
    // pgx common types might be needed for Querier if not globally defined
    // "github.com/jackc/pgx/v5"
    // "github.com/jackc/pgx/v5/pgconn"
)

// TODO: Define a shared Querier interface if not already available globally
// For now, assume pgxpool.Pool will be used directly or a local Querier.
// type Querier interface { ... }

// TransactionRepository defines the interface for transaction data persistence.
type TransactionRepository interface {
	Create(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, error)
	GetByID(ctx context.Context, id string) (*domain.Transaction, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error)
    // Add other methods like GetByReferenceID, etc. as needed
}
