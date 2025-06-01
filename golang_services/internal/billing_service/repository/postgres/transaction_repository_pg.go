package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aradsms/golang_services/internal/billing_service/domain"
	"github.com/aradsms/golang_services/internal/billing_service/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn" // For pgErr.Code
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTransactionNotFound = errors.New("transaction not found")
// No ErrDuplicateTransaction expected as ID is UUID, unless other unique constraints are added.

type pgTransactionRepository struct {
	db *pgxpool.Pool
}

// NewPgTransactionRepository creates a new instance of TransactionRepository for PostgreSQL.
func NewPgTransactionRepository(db *pgxpool.Pool) repository.TransactionRepository {
	return &pgTransactionRepository{db: db}
}

func (r *pgTransactionRepository) Create(ctx context.Context, tx *domain.Transaction) (*domain.Transaction, error) {
	tx.ID = uuid.NewString()
	tx.CreatedAt = time.Now().UTC() // Ensure UTC

	query := `
		INSERT INTO transactions (id, user_id, type, amount, currency_code, description,
		                          related_message_id, payment_gateway_txn_id, balance_after, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.Exec(ctx, query,
		tx.ID, tx.UserID, tx.Type, tx.Amount, tx.CurrencyCode, tx.Description,
		tx.RelatedMessageID, tx.PaymentGatewayTxnID, tx.BalanceAfter, tx.CreatedAt,
	)

	if err != nil {
		// No typical unique constraint errors expected here other than PK if UUID generation failed, which is unlikely.
		return nil, err
	}
	return tx, nil
}

func (r *pgTransactionRepository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	tx := &domain.Transaction{}
	query := `
		SELECT id, user_id, type, amount, currency_code, description,
		       related_message_id, payment_gateway_txn_id, balance_after, created_at
		FROM transactions WHERE id = $1
	`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&tx.ID, &tx.UserID, &tx.Type, &tx.Amount, &tx.CurrencyCode, &tx.Description,
		&tx.RelatedMessageID, &tx.PaymentGatewayTxnID, &tx.BalanceAfter, &tx.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}
	return tx, nil
}

func (r *pgTransactionRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, currency_code, description,
		       related_message_id, payment_gateway_txn_id, balance_after, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []domain.Transaction
	for rows.Next() {
		var tx domain.Transaction
		err := rows.Scan(
			&tx.ID, &tx.UserID, &tx.Type, &tx.Amount, &tx.CurrencyCode, &tx.Description,
			&tx.RelatedMessageID, &tx.PaymentGatewayTxnID, &tx.BalanceAfter, &tx.CreatedAt,
		)
		if err != nil {
			return nil, err // Or collect errors and continue
		}
		transactions = append(transactions, tx)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return transactions, nil
}
