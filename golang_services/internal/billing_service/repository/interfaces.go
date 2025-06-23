package repository

import (
	"context"
	"github.com/aradsms/golang_services/internal/billing_service/domain"
    // pgx common types might be needed for Querier if not globally defined
    "github.com/jackc/pgx/v5" // For pgx.Tx
    // "github.com/jackc/pgx/v5/pgconn"
)

// Querier defines a common interface for pgxpool.Pool and pgx.Tx for repository methods
// that need to run within or outside a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgx.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}


// TransactionRepository defines the interface for transaction data persistence.
type TransactionRepository interface {
	Create(ctx context.Context, querier Querier, transaction *domain.Transaction) (*domain.Transaction, error) // Updated to accept Querier
	GetByID(ctx context.Context, querier Querier, id string) (*domain.Transaction, error) // Updated
	GetByUserID(ctx context.Context, querier Querier, userID string, limit, offset int) ([]domain.Transaction, int, error) // Updated, added total count
    // Add other methods like GetByReferenceID, etc. as needed
}
