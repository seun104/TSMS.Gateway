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
	paymentIntentRepo  domain.PaymentIntentRepository
	paymentGateway     domain.PaymentGatewayAdapter
	tariffRepo         domain.TariffRepository // Added
	dbPool             *pgxpool.Pool
	logger             *slog.Logger
}

func NewBillingService(
	transactionRepo repository.TransactionRepository,
	userRepo userRepository.UserRepository,
	paymentIntentRepo domain.PaymentIntentRepository,
	paymentGateway domain.PaymentGatewayAdapter,
	tariffRepo domain.TariffRepository, // Added
	dbPool *pgxpool.Pool,
	logger *slog.Logger,
) *BillingService {
	return &BillingService{
		transactionRepo:   transactionRepo,
		userRepo:          userRepo,
		paymentIntentRepo: paymentIntentRepo,
		paymentGateway:    paymentGateway,
		tariffRepo:        tariffRepo, // Added
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

// DeductCreditForSMS calculates the cost of SMS messages and deducts it from the user's balance.
// It records a debit transaction.
func (s *BillingService) DeductCreditForSMS(ctx context.Context, userID uuid.UUID, numMessages int, details domain.TransactionDetails) (*domain.Transaction, error) {
	s.logger.InfoContext(ctx, "Attempting to deduct credit for SMS", "user_id", userID, "num_messages", numMessages)

	if numMessages <= 0 {
		return nil, errors.New("number of messages must be positive for credit deduction")
	}

	// 1. Calculate Cost
	cost, currency, err := s.CalculateSMSCost(ctx, userID, numMessages)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to calculate SMS cost for credit deduction", "user_id", userID, "num_messages", numMessages, "error", err)
		return nil, fmt.Errorf("cost calculation failed: %w", err)
	}
	s.logger.InfoContext(ctx, "Calculated SMS cost for deduction", "user_id", userID, "cost", cost, "currency", currency)

	// Cost here is int64 (smallest unit), but Transaction.Amount is float64.
	// And user.CreditBalance is float64. This needs careful handling of units.
	// For now, assume cost (int64) can be converted to float64 for deduction.
	// A better system would use decimal types or operate consistently in smallest units.
	costFloat := float64(cost)

	var createdTransaction *domain.Transaction
	txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
		// Fetch current user for BalanceBefore. This is illustrative.
		// In a robust system, the gRPC call to user-service to deduct credit might return balances
		// or user-service itself creates the transaction via a call back to billing-service.
		// For now, we fetch user, then call user-service to deduct, then record transaction.
		// This has distributed transaction challenges.
		user, err := s.userRepo.GetByID(ctx, userID.String()) // UserID is string for userRepo
		if err != nil {
			if errors.Is(err, userRepository.ErrUserNotFound) { // Assuming user repo has specific error
				return ErrUserNotFoundForBilling
			}
			s.logger.ErrorContext(ctx, "Failed to get user for credit deduction", "error", err, "user_id", userID)
			return fmt.Errorf("user retrieval for deduction failed: %w", err)
		}

		if user.CreditBalance < costFloat {
			s.logger.WarnContext(ctx, "Insufficient credit for SMS cost", "user_id", userID, "current_balance", user.CreditBalance, "cost", costFloat)
			return ErrInsufficientCredit
		}

		// Call user-service to update balance. This is a gRPC call.
		// Assume userRepo.Update handles the balance deduction.
		// This is where the distributed transaction complexity lies.
		// If this call succeeds but the local transaction record fails, inconsistency occurs.
		updatedUser := *user
		updatedUser.CreditBalance -= costFloat // Deduct cost
		updatedUser.UpdatedAt = time.Now().UTC()

		// Using s.userRepo.Update method, which is a gRPC client method.
		// This is not part of the local pgx.Tx transaction.
		err = s.userRepo.Update(ctx, &updatedUser)
		if err != nil {
			s.logger.ErrorContext(ctx, "Failed to deduct user credit via user service", "user_id", userID, "cost", costFloat, "error", err)
			// If this fails, we do NOT proceed to create a local transaction record.
			return fmt.Errorf("user service credit deduction failed: %w", err)
		}
		s.logger.InfoContext(ctx, "Successfully deducted credit via user service", "user_id", userID, "new_balance", updatedUser.CreditBalance)


		// Create local transaction record within the pgx.Tx
		transaction := &domain.Transaction{
			// ID will be set by Create method
			UserID:         userID.String(),
			Type:           domain.TransactionTypeDebit, // Or TransactionTypeSMSCharge
			Amount:         costFloat,                   // Store actual cost (positive value, type indicates debit)
			CurrencyCode:   currency,
			Description:    details.Description,
			RelatedMessageID: &details.ReferenceID, // Assuming ReferenceID is message ID
			BalanceBefore:  user.CreditBalance,
			BalanceAfter:   updatedUser.CreditBalance,
			// CreatedAt will be set by Create method
		}

		createdTx, err := s.transactionRepo.Create(ctx, tx, transaction)
		if err != nil {
			s.logger.ErrorContext(ctx, "Failed to create debit transaction record", "user_id", userID, "error", err)
			// IMPORTANT: User's credit was deducted by user-service, but local txn log failed.
			// This requires a compensation mechanism or robust reconciliation.
			// For now, the error will cause tx.Rollback(), but user's balance is already changed.
			return fmt.Errorf("transaction recording failed: %w", err)
		}
		createdTransaction = createdTx
		return nil // Commit local transaction
	})

	if txErr != nil {
		// If txErr is from user service call or balance check, createdTransaction will be nil.
		// If txErr is from local transaction commit or transactionRepo.Create, then also need to handle inconsistency.
		return nil, txErr
	}

	s.logger.InfoContext(ctx, "User credit deducted successfully and transaction recorded", "user_id", userID, "cost", costFloat, "transaction_id", createdTransaction.ID)
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

// CalculateSMSCost determines the cost of sending a number of SMS messages for a given user.
func (s *BillingService) CalculateSMSCost(ctx context.Context, userID uuid.UUID, numMessages int) (cost int64, currency string, err error) {
	if numMessages <= 0 {
		return 0, "", errors.New("number of messages must be positive")
	}

	tariff, err := s.tariffRepo.GetActiveUserTariff(ctx, userID)
	if err != nil {
		s.logger.ErrorContext(ctx, "Error fetching active user tariff for cost calculation", "user_id", userID, "error", err)
		return 0, "", fmt.Errorf("could not retrieve tariff for user %s: %w", userID, err)
	}

	if tariff == nil { // No user-specific tariff, try default
		s.logger.InfoContext(ctx, "No active user tariff found, attempting to use default tariff for cost calculation", "user_id", userID)
		tariff, err = s.tariffRepo.GetDefaultActiveTariff(ctx)
		if err != nil {
			s.logger.ErrorContext(ctx, "Error fetching default active tariff for cost calculation", "error", err)
			return 0, "", fmt.Errorf("could not retrieve default tariff: %w", err)
		}
		if tariff == nil {
			s.logger.ErrorContext(ctx, "No applicable tariff found for cost calculation: No user tariff and no default system tariff available.", "user_id", userID)
			return 0, "", errors.New("no applicable tariff found for cost calculation (user or default)")
		}
	}

	if !tariff.IsActive { // Should be caught by repo methods, but double check
		s.logger.ErrorContext(ctx, "Tariff selected for cost calculation is not active", "user_id", userID, "tariff_id", tariff.ID, "tariff_name", tariff.Name)
		return 0, "", fmt.Errorf("selected tariff '%s' is not active", tariff.Name)
	}

	calculatedCost := tariff.PricePerSMS * int64(numMessages)
	s.logger.InfoContext(ctx, "Calculated SMS cost",
		"user_id", userID,
		"tariff_name", tariff.Name,
		"price_per_sms", tariff.PricePerSMS,
		"num_messages", numMessages,
		"total_cost", calculatedCost,
		"currency", tariff.Currency)
	return calculatedCost, tariff.Currency, nil
}
