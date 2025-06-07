package postgres

import (
	"context"
	"errors"
	"time"
	"log/slog" // Added for logger

	"github.com/AradIT/aradsms/golang_services/internal/billing_service/domain" // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/repository" // Corrected
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	// "github.com/jackc/pgx/v5/pgconn" // For pgErr.Code - not used directly here
	"github.com/jackc/pgx/v5/pgxpool" // Keep for NewPgTransactionRepository, though methods use Querier
)

var ErrTransactionNotFound = errors.New("transaction not found")

type PgTransactionRepository struct { // Changed to exported type
	db     *pgxpool.Pool // Retain for creating new transactions if Create doesn't use Querier exclusively, or for New method
	logger *slog.Logger  // Added logger
}

// NewPgTransactionRepository creates a new instance of TransactionRepository for PostgreSQL.
func NewPgTransactionRepository(db *pgxpool.Pool, logger *slog.Logger) repository.TransactionRepository { // Return interface
	return &PgTransactionRepository{db: db, logger: logger.With("component", "transaction_repository_pg")}
}

// Create now accepts a Querier, which can be a *pgxpool.Pool or pgx.Tx
func (r *PgTransactionRepository) Create(ctx context.Context, querier repository.Querier, transaction *domain.Transaction) (*domain.Transaction, error) {
	transaction.ID = uuid.NewString() // Ensure ID is set if not already
	transaction.CreatedAt = time.Now().UTC()

	query := `
		INSERT INTO transactions (id, user_id, type, amount, currency_code, description,
		                          related_message_id, payment_gateway_txn_id, balance_before, balance_after, status, created_at, payment_intent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	// Note: Added balance_before, status, payment_intent_id to the INSERT statement.
	_, err := querier.Exec(ctx, query,
		transaction.ID, transaction.UserID, transaction.Type, transaction.Amount, transaction.CurrencyCode, transaction.Description,
		transaction.RelatedMessageID, transaction.PaymentGatewayTxnID, transaction.BalanceBefore, transaction.BalanceAfter, transaction.Status, transaction.CreatedAt, transaction.PaymentIntentID,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "Error creating transaction", "error", err, "user_id", transaction.UserID)
		return nil, fmt.Errorf("creating transaction: %w", err)
	}
	r.logger.InfoContext(ctx, "Transaction created successfully", "transaction_id", transaction.ID, "user_id", transaction.UserID)
	return transaction, nil
}

func (r *PgTransactionRepository) GetByID(ctx context.Context, querier repository.Querier, id string) (*domain.Transaction, error) {
	transaction := &domain.Transaction{}
	query := `
		SELECT id, user_id, type, amount, currency_code, description,
		       related_message_id, payment_gateway_txn_id, balance_before, balance_after, status, created_at, payment_intent_id
		FROM transactions WHERE id = $1
	`
	// Note: Added balance_before, status, payment_intent_id to SELECT
	err := querier.QueryRow(ctx, query, id).Scan(
		&transaction.ID, &transaction.UserID, &transaction.Type, &transaction.Amount, &transaction.CurrencyCode, &transaction.Description,
		&transaction.RelatedMessageID, &transaction.PaymentGatewayTxnID, &transaction.BalanceBefore, &transaction.BalanceAfter, &transaction.Status, &transaction.CreatedAt, &transaction.PaymentIntentID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.InfoContext(ctx, "Transaction not found by ID", "transaction_id", id)
			return nil, ErrTransactionNotFound
		}
		r.logger.ErrorContext(ctx, "Error scanning transaction by ID", "transaction_id", id, "error", err)
		return nil, fmt.Errorf("scanning transaction by ID %s: %w", id, err)
	}
	return transaction, nil
}

func (r *PgTransactionRepository) GetByUserID(ctx context.Context, querier repository.Querier, userID string, limit, offset int) ([]domain.Transaction, int, error) {
	countQuery := `SELECT COUNT(*) FROM transactions WHERE user_id = $1`
	var totalCount int
	if err := querier.QueryRow(ctx, countQuery, userID).Scan(&totalCount); err != nil {
		r.logger.ErrorContext(ctx, "Error counting transactions by UserID", "user_id", userID, "error", err)
		return nil, 0, fmt.Errorf("counting transactions for user %s: %w", userID, err)
	}

	if totalCount == 0 {
		return []domain.Transaction{}, 0, nil
	}

	query := `
		SELECT id, user_id, type, amount, currency_code, description,
		       related_message_id, payment_gateway_txn_id, balance_before, balance_after, status, created_at, payment_intent_id
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := querier.Query(ctx, query, userID, limit, offset)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error querying transactions by UserID", "user_id", userID, "error", err)
		return nil, 0, fmt.Errorf("querying transactions for user %s: %w", userID, err)
	}
	defer rows.Close()

	var transactions []domain.Transaction
	for rows.Next() {
		var transaction domain.Transaction
		// Note: Added balance_before, status, payment_intent_id to SELECT
		err := rows.Scan(
			&transaction.ID, &transaction.UserID, &transaction.Type, &transaction.Amount, &transaction.CurrencyCode, &transaction.Description,
			&transaction.RelatedMessageID, &transaction.PaymentGatewayTxnID, &transaction.BalanceBefore, &transaction.BalanceAfter, &transaction.Status, &transaction.CreatedAt, &transaction.PaymentIntentID,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Error scanning transaction row for UserID", "user_id", userID, "error", err)
			return nil, 0, fmt.Errorf("scanning transaction for user %s: %w", userID, err)
		}
		transactions = append(transactions, transaction)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error after iterating transaction rows for UserID", "user_id", userID, "error", err)
		return nil, 0, fmt.Errorf("iterating transactions for user %s: %w", userID, err)
	}

	return transactions, totalCount, nil
}
