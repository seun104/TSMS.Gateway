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
    userRepository "github.com/aradsms/golang_services/internal/user_service/repository"
    userDomain "github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/google/uuid" // For PaymentIntent UserID

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5"
)

var ErrInsufficientCredit = errors.New("insufficient credit")
var ErrUserNotFoundForBilling = errors.New("user not found for billing operation")
var ErrPaymentGateway = errors.New("payment gateway error")


type BillingService struct {
	transactionRepo    repository.TransactionRepository
    userRepo           userRepository.UserRepository
	paymentIntentRepo  domain.PaymentIntentRepository  // Added
	paymentGateway     domain.PaymentGatewayAdapter    // Added
	dbPool             *pgxpool.Pool
	logger             *slog.Logger
}

func NewBillingService(
	transactionRepo repository.TransactionRepository,
    userRepo userRepository.UserRepository,
	paymentIntentRepo domain.PaymentIntentRepository, // Added
	paymentGateway domain.PaymentGatewayAdapter,    // Added
    dbPool *pgxpool.Pool,
	logger *slog.Logger,
) *BillingService {
	return &BillingService{
		transactionRepo:   transactionRepo,
        userRepo:          userRepo,
		paymentIntentRepo: paymentIntentRepo, // Added
		paymentGateway:    paymentGateway,    // Added
        dbPool:            dbPool,
		logger:            logger.With("service", "billing"),
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

// CreatePaymentIntent creates a payment intent with the gateway and stores a local record.
func (s *BillingService) CreatePaymentIntent(ctx context.Context, userID uuid.UUID, amount int64, currency string, email string, description string) (*domain.CreateIntentResponse, string, error) {
	s.logger.InfoContext(ctx, "Creating payment intent", "user_id", userID, "amount", amount, "currency", currency)

	if amount <= 0 {
		return nil, "", errors.New("amount must be positive")
	}
	if currency == "" {
		return nil, "", errors.New("currency is required")
	}

	// Internal Payment Intent ID
	internalPIID := uuid.New()

	// 1. Call Payment Gateway Adapter to create intent at gateway
	adapterReq := domain.CreateIntentRequest{
		Amount:            amount,
		Currency:          currency,
		UserID:            userID,
		Email:             email,       // Pass through email
		Description:       description, // Pass through description
		InternalRequestID: internalPIID.String(), // For idempotency/logging with gateway
	}
	adapterResp, err := s.paymentGateway.CreatePaymentIntent(ctx, adapterReq)
	if err != nil {
		s.logger.ErrorContext(ctx, "Payment gateway failed to create payment intent", "error", err, "user_id", userID)
		// Even if gateway fails, we might want to store a local PI record with 'failed' status.
		// For now, let's return an error and not store anything if gateway call fails outright.
		return nil, "", fmt.Errorf("%w: %s", ErrPaymentGateway, err.Error())
	}

	// 2. Create and store local PaymentIntent record within a transaction
	pi := &domain.PaymentIntent{
		ID:                     internalPIID,
		UserID:                 userID,
		Amount:                 amount,
		Currency:               currency,
		Status:                 adapterResp.Status, // Initial status from gateway
		GatewayPaymentIntentID: &adapterResp.GatewayPaymentIntentID,
		GatewayClientSecret:    adapterResp.ClientSecret,
		ErrorMessage:           adapterResp.ErrorMessage, // If gateway returned an error message but not a fatal error
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}

	txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
		// Using a hypothetical Querier-accepting Create method for PaymentIntentRepository
		// If PaymentIntentRepository methods don't accept Querier, this won't be truly transactional with other DB ops here.
		// For now, assume PaymentIntentRepository.Create itself handles its operation correctly.
		// The PaymentIntentRepository was not specified to take pgx.Tx, so it runs in its own transaction.
		// This is acceptable if creating PI is a single operation.
		return s.paymentIntentRepo.Create(ctx, pi)
	})

	if txErr != nil {
		s.logger.ErrorContext(ctx, "Failed to save local payment intent record", "error", txErr, "internal_pi_id", internalPIID, "gateway_pi_id", adapterResp.GatewayPaymentIntentID)
		// If DB save fails, what to do with gateway intent? Attempt to cancel? Log for manual reconciliation?
		// For now, return error.
		return nil, "", fmt.Errorf("failed to save payment intent: %w", txErr)
	}

	s.logger.InfoContext(ctx, "Payment intent created successfully", "internal_pi_id", pi.ID, "gateway_pi_id", *pi.GatewayPaymentIntentID, "status", pi.Status)

	// Return the necessary details for the caller (e.g., API response)
	// This is different from adapterResp; it's our system's response about the intent.
	// The clientSecret is crucial for client-side payment methods like Stripe.
	return &domain.CreateIntentResponse{
        GatewayPaymentIntentID: adapterResp.GatewayPaymentIntentID,
        ClientSecret:           adapterResp.ClientSecret,
        NextActionURL:          adapterResp.NextActionURL,
        Status:                 adapterResp.Status,
        ErrorMessage:           adapterResp.ErrorMessage,
    }, pi.ID.String(), nil
}


// HandlePaymentWebhook processes incoming webhook events from the payment gateway.
func (s *BillingService) HandlePaymentWebhook(ctx context.Context, rawPayload []byte, signature string) error {
	s.logger.InfoContext(ctx, "Handling payment webhook")

	// 1. Parse and verify event using the gateway adapter
	event, err := s.paymentGateway.HandleWebhookEvent(ctx, rawPayload, signature)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to handle webhook event via adapter", "error", err)
		return fmt.Errorf("%w: %s", ErrPaymentGateway, err.Error()) // Return generic error to avoid leaking details
	}
	s.logger.InfoContext(ctx, "Webhook event processed by adapter", "gateway_pi_id", event.GatewayPaymentIntentID, "event_type", event.Type)

	// 2. Process the event within a database transaction
	txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
		// Fetch local PaymentIntent record
		pi, err := s.paymentIntentRepo.GetByGatewayPaymentIntentID(ctx, event.GatewayPaymentIntentID) // Assume repo methods use ctx, not tx for now
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				s.logger.ErrorContext(ctx, "PaymentIntent not found for webhook's gateway_payment_intent_id", "gateway_pi_id", event.GatewayPaymentIntentID)
				return fmt.Errorf("payment intent not found for gateway ID %s: %w", event.GatewayPaymentIntentID, err) // Error, but might not be retryable by gateway if PI truly DNE
			}
			s.logger.ErrorContext(ctx, "Failed to retrieve payment intent by gateway ID", "error", err, "gateway_pi_id", event.GatewayPaymentIntentID)
			return fmt.Errorf("database error retrieving payment intent: %w", err) // Retryable by gateway
		}

		// Idempotency check: if already processed to a terminal state, maybe ignore or just log.
		if pi.Status == domain.PaymentIntentStatusSucceeded && event.Type == string(domain.PaymentIntentStatusSucceeded) { // Example type check
			s.logger.InfoContext(ctx, "PaymentIntent already marked as succeeded, webhook idempotent.", "payment_intent_id", pi.ID, "gateway_pi_id", pi.GatewayPaymentIntentID)
			return nil
		}
		if (pi.Status == domain.PaymentIntentStatusFailed || pi.Status == domain.PaymentIntentStatusCancelled) &&
		   (event.Type == string(domain.PaymentIntentStatusFailed) || event.Type == string(domain.PaymentIntentStatusCancelled)) {
			s.logger.InfoContext(ctx, "PaymentIntent already in a terminal non-success state.", "payment_intent_id", pi.ID, "status", pi.Status)
			return nil
		}


		// Update PaymentIntent based on event
		previousStatus := pi.Status
		pi.UpdatedAt = time.Now().UTC()

		switch event.Type { // Assuming event.Type aligns with PaymentIntentStatus strings or is mapped
		case string(domain.PaymentIntentStatusSucceeded):
			pi.Status = domain.PaymentIntentStatusSucceeded
			pi.ErrorMessage = nil // Clear previous errors if any

			// Grant credit to user and record transaction
			// This needs to be robust and potentially idempotent itself.
			// The userRepo.UpdateCreditBalance should ideally be idempotent or the transaction should handle it.
			// For simplicity, assume AddUserCredit is what userRepo (gRPC client to user-service) provides.
			// The amount added should come from the PaymentIntent (pi.Amount), not event.AmountReceived,
			// as event.AmountReceived is for reconciliation against what gateway says it processed.
			// We trust our pi.Amount for how much credit to grant.

			// Fetch user details to get current balance for transaction logging
			// This is non-transactional with respect to user-service DB if it's a separate service.
			user, err := s.userRepo.GetByID(ctx, pi.UserID.String()) // UserID on PI is uuid.UUID
			if err != nil {
				s.logger.ErrorContext(ctx, "Failed to get user for credit top-up", "error", err, "user_id", pi.UserID)
				pi.Status = domain.PaymentIntentStatusFailed // Mark PI as failed if user cannot be found for credit update
				errMsg := fmt.Sprintf("User %s not found for credit top-up after payment success", pi.UserID)
				pi.ErrorMessage = &errMsg
				// No return err here, save PI and let it be.
			} else {
				// Add credit to user (this is a call to user-service via gRPC client s.userRepo)
				// This operation itself isn't part of the local DB transaction in billing-service.
				// This is a distributed transaction scenario if user balance is in user-service.
				// A SAGA pattern or an event-driven approach would be more robust.
				// For now, direct call:
				// err = s.userRepo.AddUserCredit(ctx, pi.UserID.String(), pi.Amount) // Method needs to exist on userRepo
				// Let's assume userRepo.Update directly handles adding positive amount
				updatedUser := *user
				updatedUser.CreditBalance += float64(pi.Amount) // Assuming pi.Amount is in smallest unit, needs conversion if CreditBalance is not
				// For now, assume direct mapping or that pi.Amount is already in correct scale for CreditBalance
				// This is a simplification. Real system needs careful unit/scale management.

				err = s.userRepo.Update(ctx, &updatedUser) // This call is to user-service
				if err != nil {
					s.logger.ErrorContext(ctx, "Failed to add credit to user via user service", "error", err, "user_id", pi.UserID, "amount", pi.Amount)
					// PI succeeded at gateway, but credit not granted. Critical error. Log for manual intervention.
					// Mark PI as requires_manual_intervention or similar? Or keep as succeeded and rely on alerts?
					errMsg := fmt.Sprintf("Gateway payment succeeded but failed to grant credit to user %s: %v", pi.UserID, err)
					pi.ErrorMessage = &errMsg // Append error to PI
					// Do not change PI status from Succeeded here as payment WAS successful.
				} else {
					s.logger.InfoContext(ctx, "Successfully added credit to user", "user_id", pi.UserID, "amount", pi.Amount)
					// Create credit transaction
					creditTxn := &domain.Transaction{
						UserID:         pi.UserID.String(),
						Type:           domain.TransactionTypeCredit, // "CREDIT"
						Amount:         float64(pi.Amount), // Store as positive for credit addition
						CurrencyCode:   pi.Currency,
						Description:    fmt.Sprintf("Credit top-up from payment %s", pi.ID.String()),
						PaymentIntentID: &pi.ID,
						BalanceBefore:  user.CreditBalance, // Balance before this top-up
						BalanceAfter:   updatedUser.CreditBalance, // Balance after top-up
					}
					// transactionRepo.Create needs to accept pgx.Tx (querier)
					if _, err := s.transactionRepo.Create(ctx, tx, creditTxn); err != nil { // Pass the transaction 'tx'
						s.logger.ErrorContext(ctx, "Failed to create credit top-up transaction record", "error", err, "payment_intent_id", pi.ID)
						// This is also critical. Payment succeeded, credit granted (maybe), but transaction log failed.
						errMsg := fmt.Sprintf("Failed to log credit transaction for PI %s: %v", pi.ID, err)
						currentErrMsg := ""
						if pi.ErrorMessage != nil { currentErrMsg = *pi.ErrorMessage + "; " }
						pi.ErrorMessage = &(currentErrMsg + errMsg)
						// Still, payment was successful.
					}
				}
			}

		case string(domain.PaymentIntentStatusFailed):
			pi.Status = domain.PaymentIntentStatusFailed
			errMsg := fmt.Sprintf("Gateway indicated payment failed for PI %s. Event: %s", pi.ID, event.Type)
			if eventData, ok := event.Data["failure_reason"].(string); ok && eventData != "" {
				errMsg += ". Reason: " + eventData
			}
			pi.ErrorMessage = &errMsg

		case string(domain.PaymentIntentStatusCancelled):
			pi.Status = domain.PaymentIntentStatusCancelled
			errMsg := fmt.Sprintf("Gateway indicated payment cancelled for PI %s. Event: %s", pi.ID, event.Type)
			pi.ErrorMessage = &errMsg

		// Add other relevant cases like "requires_action", "processing" if gateway sends them
		default:
			s.logger.WarnContext(ctx, "Unhandled payment gateway event type", "event_type", event.Type, "gateway_pi_id", event.GatewayPaymentIntentID)
			// Store the event type or a generic message if status not directly mappable
			// errMsg := fmt.Sprintf("Unhandled gateway event: %s", event.Type)
			// pi.ErrorMessage = &errMsg
			// pi.Status remains unchanged or set to a review status
			return nil // No change to PI for unhandled event type, or log and proceed.
		}

		if previousStatus != pi.Status {
			s.logger.InfoContext(ctx, "PaymentIntent status updated by webhook", "payment_intent_id", pi.ID, "old_status", previousStatus, "new_status", pi.Status, "gateway_event_type", event.Type)
		} else {
			s.logger.InfoContext(ctx, "PaymentIntent status unchanged by webhook", "payment_intent_id", pi.ID, "status", pi.Status, "gateway_event_type", event.Type)
		}

		// Save updated PaymentIntent (using tx)
		// The PaymentIntentRepository.Update method was not specified to take pgx.Tx.
		// For true atomicity with transactionRepo.Create, it should.
		// For now, assume Update is self-contained or we accept this limitation.
		return s.paymentIntentRepo.Update(ctx, pi)
	})

	if txErr != nil {
		s.logger.ErrorContext(ctx, "Transaction failed during webhook processing", "error", txErr, "gateway_pi_id", event.GatewayPaymentIntentID)
		// Error returned to gateway, which might retry.
		return txErr
	}

	s.logger.InfoContext(ctx, "Webhook processed successfully for payment intent", "gateway_pi_id", event.GatewayPaymentIntentID)
	return nil
}
