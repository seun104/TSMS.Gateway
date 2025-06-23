package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
    // "database/sql" // For sql.NullString equivalent if needed, or handle pointers


	"github.com/aradsms/golang_services/internal/billing_service/domain"
	"github.com/aradsms/golang_services/internal/billing_service/repository"
	"github.com/aradsms/golang_services/internal/platform/config" // Assuming AppConfig is shared or billing has its own
	// user_service_client "github.com/aradsms/golang_services/internal/billing_service/adapters/grpc_clients" // If calling user-service for credit
)

var ErrInsufficientCredit = errors.New("insufficient credit")
var ErrUserNotFoundForBilling = errors.New("user not found for billing operation")

// BillingService provides core billing logic.
type BillingService struct {
	transactionRepo repository.TransactionRepository
	// userCreditGetter UserCreditGetter // Interface to get user credit (could be user-service gRPC client)
	// userCreditUpdater UserCreditUpdater // Interface to update user credit (could be user-service gRPC client)
	logger          *slog.Logger
    appConfig       *config.AppConfig // For currency code default etc.
}

// UserCreditGetter defines an interface to get user credit.
// This would be implemented by a gRPC client to the user-service in a full setup.
type UserCreditGetter interface {
    GetUserCredit(ctx context.Context, userID string) (balance float64, currencyCode string, err error)
}

// UserCreditUpdater defines an interface to update user credit.
type UserCreditUpdater interface {
    UpdateUserCredit(ctx context.Context, userID string, newBalance float64) error
}


// NewBillingService creates a new BillingService.
func NewBillingService(
	transactionRepo repository.TransactionRepository,
	// userCreditGetter UserCreditGetter,
    // userCreditUpdater UserCreditUpdater,
	logger *slog.Logger,
    appConfig *config.AppConfig,
) *BillingService {
	return &BillingService{
		transactionRepo: transactionRepo,
		// userCreditGetter: userCreditGetter,
        // userCreditUpdater: userCreditUpdater,
		logger:          logger.With("service", "billing"),
        appConfig:       appConfig,
	}
}

// CheckAndDeductCredit checks if a user has enough credit and deducts the amount if so.
// It creates a transaction record for the deduction.
// For Phase 2, this method will assume credit is managed/updated directly via user-service
// or that user-service calls this service after deducting credit to just record the transaction.
// Let's assume for now this service IS RESPONSIBLE for orchestrating the check & debit with user-service,
// and then records its own transaction. This requires a gRPC client to user-service.

// For simplicity in this step (as user-service gRPC doesn't have UpdateCredit yet):
// We will simulate the credit check and update, and focus on creating the transaction.
// In a real scenario, it would call user-service to get credit, then call user-service to update credit,
// and only if both succeed, create the local transaction. This should be transactional.
func (s *BillingService) CheckAndDeductCredit(
	ctx context.Context,
	userID string,
	amountToDeduct float64, // Should always be positive
	transactionType domain.TransactionType,
	description string,
	relatedMessageID *string,
) (*domain.Transaction, error) {
	s.logger.InfoContext(ctx, "CheckAndDeductCredit called", "userID", userID, "amount", amountToDeduct)

	if amountToDeduct <= 0 {
		return nil, errors.New("deduction amount must be positive")
	}

    // --- SIMULATED CREDIT CHECK AND DEDUCTION ---
    // TODO: Replace with actual gRPC calls to user-service:
    // 1. Get current credit from user-service.
    // currentBalance, _, err := s.userCreditGetter.GetUserCredit(ctx, userID)
    // if err != nil {
    //     s.logger.ErrorContext(ctx, "Failed to get user credit", "error", err, "userID", userID)
    //     return nil, fmt.Errorf("failed to retrieve user credit: %w", err)
    // }
    // if currentBalance < amountToDeduct {
    //     return nil, ErrInsufficientCredit
    // }
    // newBalance := currentBalance - amountToDeduct
    // err = s.userCreditUpdater.UpdateUserCredit(ctx, userID, newBalance)
    // if err != nil {
    //    s.logger.ErrorContext(ctx, "Failed to update user credit", "error", err, "userID", userID, "newBalance", newBalance)
    //    return nil, fmt.Errorf("failed to update user credit: %w", err)
    // }
    // --- END SIMULATION ---
    s.logger.InfoContext(ctx, "[SIMULATED] User credit check and deduction successful", "userID", userID, "deducted", amountToDeduct)
    simulatedBalanceAfter := 100.0 - amountToDeduct // Purely for example


	transaction := &domain.Transaction{
		UserID:           userID,
		Type:             transactionType,
		Amount:           -amountToDeduct, // Store deductions as negative
		CurrencyCode:     s.appConfig.DefaultCurrency, // Get from config or user's profile
		Description:      description,
		RelatedMessageID: relatedMessageID,
        BalanceAfter:     &simulatedBalanceAfter, // Store the simulated balance
	}

	createdTx, err := s.transactionRepo.Create(ctx, transaction)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to create transaction record", "error", err, "userID", userID)
		// TODO: If this fails, we need a compensation mechanism for the credit deduction in user-service.
		// This highlights the need for distributed transaction patterns (e.g., Saga) if services are separate.
		return nil, fmt.Errorf("failed to record transaction: %w", err)
	}

	s.logger.InfoContext(ctx, "Credit deducted and transaction recorded", "userID", userID, "transactionID", createdTx.ID, "amount", createdTx.Amount)
	return createdTx, nil
}
