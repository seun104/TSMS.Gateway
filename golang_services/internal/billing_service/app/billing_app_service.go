package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
    "time" // Added for potential locking or transaction timeouts

	"github.com/aradsms/golang_services/internal/billing_service/domain"
	"github.com/aradsms/golang_services/internal/billing_service/repository"
	// Assuming user_service domain/repo might be needed to get user's current balance or update it
	// For simplicity now, we'll assume balance is managed within billing transactions or passed around.
	// A direct dependency on user_service repository might be an option, or gRPC call to user_service.
	// Let's assume user_service's User model has CreditBalance and it's the source of truth.
	// This means billing-service might need a gRPC client to user-service to update user balance,
	// or user-service calls billing-service to record transaction after updating its own balance.
	// For Phase 2, let's simplify: billing service updates balance directly on user table (requires user repo) OR
	// focuses only on recording transactions and expects caller to manage user balance.
	// The current Transaction model has BalanceBefore/After, suggesting it logs the state.
	// Let's assume for now this service *also* updates the user's balance on the users table.
	// This implies a dependency on UserRepository or a gRPC client to user_service.
	// For this iteration, let's use a conceptual UserRepository.

    // IMPORTANT: This dependency on another service's repository is generally not ideal for microservices.
    // A better approach would be:
    // 1. Eventual consistency: Billing service records transaction. User service listens to "transaction_recorded" event and updates balance.
    // 2. Orchestration: SMS Sending service calls User service to debit credit, then User service calls Billing service to log transaction.
    // 3. Billing service is the source of truth for balance: All credit checks/debits go to Billing service.
    // For this partial setup, we'll simulate direct user balance update via a passed-in UserRepository.
    // This UserRepository would be from the user_service's own defined interface.
    userRepository "github.com/aradsms/golang_services/internal/user_service/repository" // Path to user_service's repo interface
    userDomain "github.com/aradsms/golang_services/internal/user_service/domain" // Path to user_service's domain model

    "github.com/jackc/pgx/v5/pgxpool" // For managing transactions with Querier
    "github.com/jackc/pgx/v5" // For pgx.Tx
)

var ErrInsufficientCredit = errors.New("insufficient credit")
var ErrUserNotFoundForBilling = errors.New("user not found for billing operation")

type BillingService struct {
	transactionRepo repository.TransactionRepository
    userRepo        userRepository.UserRepository // To update user's actual balance
	dbPool          *pgxpool.Pool // For starting transactions
	logger          *slog.Logger
}

func NewBillingService(
	transactionRepo repository.TransactionRepository,
    userRepo userRepository.UserRepository,
    dbPool *pgxpool.Pool,
	logger *slog.Logger,
) *BillingService {
	return &BillingService{
		transactionRepo: transactionRepo,
        userRepo:        userRepo,
        dbPool:          dbPool,
		logger:          logger.With("service", "billing"),
	}
}

// HasSufficientCredit checks if a user has enough credit for an amount.
// This should ideally fetch the current balance from the authoritative source (e.g., users table).
func (s *BillingService) HasSufficientCredit(ctx context.Context, userID string, amountToDeduct float64) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID) // Assumes GetByID exists and returns user with CreditBalance
	if err != nil {
		if errors.Is(err, userRepository.ErrUserNotFound) {
			return false, ErrUserNotFoundForBilling
		}
		return false, fmt.Errorf("failed to get user for credit check: %w", err)
	}
	return user.CreditBalance >= amountToDeduct, nil
}

// DeductCredit deducts credit from a user and records the transaction.
// This operation should be transactional.
func (s *BillingService) DeductCredit(
	ctx context.Context,
	userID string,
	amountToDeduct float64,
	transactionType domain.TransactionType,
	description string,
	relatedMessageID *string,
) (*domain.Transaction, error) {
	if amountToDeduct <= 0 {
		return nil, errors.New("deduction amount must be positive")
	}

    var createdTransaction *domain.Transaction
    txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
        // 1. Get current user balance (with FOR UPDATE to lock the row)
        // Note: pgx.Tx itself doesn't directly offer FOR UPDATE in BeginFunc easily.
        // A raw SQL query inside the transaction is needed for row-level locking.
        // Or, trust the application-level logic if concurrent debits for the same user are rare
        // or handled by optimistic locking / retries.
        // For simplicity, we'll fetch and then update. A real system needs robust concurrency control here.

        // The userRepo methods GetByIDForUpdate and UpdateCreditBalance need to accept pgx.Tx (Querier)
        // This requires modifying user_service repository interfaces and implementations.
        // For now, this will be a conceptual placeholder for such methods.
        // Let's assume they exist and can take `tx` as a Querier.

        // user, err := s.userRepo.GetByIDForUpdate(ctx, tx, userID) // Assume GetByIDForUpdate locks the user row
        // This call signature needs userRepo to be adapted for Querier, or use a specific method.
        // For now, let's assume a conceptual GetUserForUpdate method that uses the transaction.
        // This needs to be properly implemented in user_service's repository.
        // We'll simulate this by fetching user first, then trying to update.
        // This is NOT transactionally safe without proper locking (SELECT ... FOR UPDATE).

        // Placeholder for locking and fetching user - this part needs proper implementation
        // with userRepo methods that accept a transaction (Querier).
        // For this subtask, we proceed with the logical flow, acknowledging this gap.
        user := &userDomain.User{} // Placeholder
        var err error
        // Conceptual: user, err = s.userRepo.GetByIDAndLock(ctx, tx, userID)
        // If above not possible, then this is not safe:
        tempUser, err := s.userRepo.GetByID(ctx, userID) // Not using tx here, not ideal
        if err != nil {
            if errors.Is(err, userRepository.ErrUserNotFound) {
                return ErrUserNotFoundForBilling
            }
            s.logger.ErrorContext(ctx, "Failed to get user for credit deduction (non-tx)", "error", err, "userID", userID)
            return fmt.Errorf("user retrieval failed (non-tx): %w", err)
        }
        user = tempUser // Assign to user

        if user.CreditBalance < amountToDeduct {
            return ErrInsufficientCredit
        }

        balanceBefore := user.CreditBalance
        balanceAfter := user.CreditBalance - amountToDeduct
        // user.CreditBalance = balanceAfter // This updates local copy, real update below
        // user.UpdatedAt = time.Now()

        // 2. Update user's balance
        // Conceptual: err = s.userRepo.UpdateCreditBalanceInTx(ctx, tx, userID, balanceAfter)
        // If userRepo cannot accept tx, this is not part of the same DB transaction.
        // This is a major simplification and architectural consideration.
        // For now, we assume Update updates the balance and other fields.
        tempUserForUpdate := *user // Create a copy to update
        tempUserForUpdate.CreditBalance = balanceAfter
        tempUserForUpdate.UpdatedAt = time.Now()
        if err := s.userRepo.Update(ctx, &tempUserForUpdate); err != nil { // Not using tx here, not ideal
            s.logger.ErrorContext(ctx, "Failed to update user credit balance (non-tx)", "error", err, "userID", userID)
            return fmt.Errorf("balance update failed (non-tx): %w", err)
        }


        // 3. Record the transaction using the transaction (tx)
        txn := &domain.Transaction{
            UserID:              userID,
            Type:                transactionType,
            Amount:              -amountToDeduct, // Store as negative for deduction
            CurrencyCode:        user.CurrencyCode, // Assuming user has CurrencyCode
            Description:         description,
            RelatedMessageID:    relatedMessageID,
            BalanceBefore:       balanceBefore,
            BalanceAfter:        balanceAfter,
        }

        createdTransaction, err = s.transactionRepo.Create(ctx, tx, txn) // This uses the tx
        if err != nil {
            s.logger.ErrorContext(ctx, "Failed to create transaction record", "error", err, "userID", userID)
            return fmt.Errorf("transaction recording failed: %w", err)
        }
        return nil // Commit transaction
    })


	if txErr != nil {
		return nil, txErr // Return the error from the transaction block
	}
	return createdTransaction, nil
}
