package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aradsms/golang_services/api/proto/billingservice" // gRPC client for billing
	"github.com/aradsms/golang_services/internal/core_sms/domain"
	"github.com/aradsms/golang_services/internal/platform/messagebroker" // NATS
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository"
	// "github.com/google/uuid" // Not directly used here, but OutboxMessage.ID might be UUID string
	"github.com/nats-io/nats.go"
    "github.com/jackc/pgx/v5/pgxpool" // For DB transactions
    "github.com/jackc/pgx/v5"       // For pgx.Tx
)

// NATSJobPayload defines the structure of the message expected from NATS.
type NATSJobPayload struct {
	OutboxMessageID string `json:"outbox_message_id"` // ID of the pre-created OutboxMessage
	// Alternatively, pass all details directly if OutboxMessage is created here:
	// UserID          string  `json:"user_id"`
	// SenderID        string  `json:"sender_id"`
	// Recipient       string  `json:"recipient"`
	// Content         string  `json:"content"`
	// UserData        *string `json:"user_data,omitempty"`
}

// SMSSendingAppService orchestrates the SMS sending process.
type SMSSendingAppService struct {
	outboxRepo          repository.OutboxRepository
	providers           map[string]provider.SMSSenderProvider // Map of available providers
	defaultProviderName string                                // Name of the default provider
	billingClient       billingservice.BillingInternalServiceClient
	natsClient          *messagebroker.NatsClient
	dbPool              *pgxpool.Pool // For starting transactions
	logger              *slog.Logger
	router              *Router            // Added router
	natsSub             *nats.Subscription // To manage the NATS subscription
}

// NewSMSSendingAppService creates a new SMSSendingAppService.
func NewSMSSendingAppService(
	outboxRepo repository.OutboxRepository,
	providers map[string]provider.SMSSenderProvider,
	defaultProviderName string,
	billingClient billingservice.BillingInternalServiceClient,
	natsClient *messagebroker.NatsClient,
	dbPool *pgxpool.Pool,
	logger *slog.Logger,
	router *Router, // Added router
) *SMSSendingAppService {
	return &SMSSendingAppService{
		outboxRepo:          outboxRepo,
		providers:           providers,
		defaultProviderName: defaultProviderName,
		billingClient:       billingClient,
		natsClient:          natsClient,
		dbPool:              dbPool,
		logger:              logger.With("service", "sms_sending_app"),
		router:              router, // Added router
	}
}

// StartConsumingJobs subscribes to the NATS subject for SMS jobs.
func (s *SMSSendingAppService) StartConsumingJobs(ctx context.Context, subject, queueGroup string) error {
	if s.natsClient == nil {
		return errors.New("NATS client not initialized in SMSSendingAppService")
	}
	s.logger.Info("Starting NATS job consumer", "subject", subject, "queue_group", queueGroup)

	msgHandler := func(msg *nats.Msg) {
		s.logger.Info("Received NATS job", "subject", msg.Subject, "data_len", len(msg.Data))
		var job NATSJobPayload
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			s.logger.Error("Failed to unmarshal NATS job payload", "error", err, "data", string(msg.Data))
			// Nack? Dead-letter queue? For now, just log.
			return
		}

		// Process the job within a new context to handle potential timeouts per job
		jobCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Example timeout
		defer cancel()

		if err := s.processSMSJob(jobCtx, job); err != nil {
			s.logger.Error("Failed to process SMS job", "error", err, "outbox_message_id", job.OutboxMessageID)
			// Handle retry logic or dead-letter queue based on error type if needed
		}
	}

	var err error
	s.natsSub, err = s.natsClient.Subscribe(ctx, subject, queueGroup, msgHandler)
	if err != nil {
		return fmt.Errorf("failed to subscribe to NATS subject '%s': %w", subject, err)
	}
	return nil
}

// processSMSJob handles the logic for a single SMS job.
func (s *SMSSendingAppService) processSMSJob(ctx context.Context, job NATSJobPayload) error {
	s.logger.InfoContext(ctx, "Processing SMS job", "outbox_message_id", job.OutboxMessageID)

    var outboxMsg *domain.OutboxMessage
    var err error

    // Start a database transaction
    txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
        // 1. Fetch the OutboxMessage (assuming it was created by public-api-service)
        // If not, then create it here based on full payload from NATS.
        // For this flow, we assume it's pre-created and ID is passed.
        outboxMsg, err = s.outboxRepo.GetByID(ctx, tx, job.OutboxMessageID)
        if err != nil {
            s.logger.ErrorContext(ctx, "Failed to get outbox message", "error", err, "id", job.OutboxMessageID)
            return fmt.Errorf("outbox message not found: %w", err) // Stop processing if not found
        }

        // Check if already processed beyond "queued" (idempotency)
        if outboxMsg.Status != domain.MessageStatusQueued {
            s.logger.WarnContext(ctx, "SMS job already processed or in invalid state", "id", job.OutboxMessageID, "status", outboxMsg.Status)
            return nil // Acknowledge NATS message, do not reprocess
        }

        // Update status to "processing"
        processingTime := time.Now()
        outboxMsg.Status = domain.MessageStatusProcessing
        outboxMsg.ProcessedAt = &processingTime
        if errStatusUpdate := s.outboxRepo.UpdateStatus(ctx, tx, outboxMsg.ID, outboxMsg.Status, nil, nil, nil); errStatusUpdate != nil {
             s.logger.ErrorContext(ctx, "Failed to update outbox status to processing", "error", errStatusUpdate, "id", outboxMsg.ID)
             return fmt.Errorf("db update (processing) failed: %w", errStatusUpdate)
        }

        // 2. Check & Deduct Credit via Billing Service
        // TODO: Calculate actual cost/segments for amountToDeduct
        amountToDeduct := 1.0 // Placeholder: 1 unit of credit per SMS
        deductReq := &billingservice.DeductCreditRequest{
            UserId:           outboxMsg.UserID,
            AmountToDeduct:   amountToDeduct,
            TransactionType:  string(domain.TransactionTypeSMSCharge), // domain.TransactionType defined in billing_service
            Description:      fmt.Sprintf("SMS charge for message to %s", outboxMsg.Recipient),
            RelatedMessageId: &outboxMsg.ID,
        }
        s.logger.InfoContext(ctx, "Calling billing service to deduct credit", "userID", outboxMsg.UserID, "amount", amountToDeduct)

        billingCtx, billingCancel := context.WithTimeout(ctx, 30*time.Second) // Timeout for gRPC call
        defer billingCancel()
        _, errBilling := s.billingClient.DeductCredit(billingCtx, deductReq)
        if errBilling != nil {
            s.logger.ErrorContext(ctx, "Failed to deduct credit via billing service", "error", errBilling, "userID", outboxMsg.UserID)
            errMsg := fmt.Sprintf("Billing error: %v", errBilling)
            sentTime := time.Now() // Time of failure
            // Use existing tx for this update
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, domain.MessageStatusFailedProviderSubmission, nil, nil, sentTime, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox after billing failure", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf("credit deduction failed: %w", errBilling)
        }
        s.logger.InfoContext(ctx, "Credit deducted successfully", "userID", outboxMsg.UserID)

        // 3. Send SMS via Provider
        sendDetails := provider.SendRequestDetails{
            InternalMessageID: outboxMsg.ID,
            SenderID:          outboxMsg.SenderID,
            Recipient:         outboxMsg.Recipient,
            Content:           outboxMsg.Content,
            UserData:          outboxMsg.UserData,
        }

        // Select provider using the router
        userIDStr := outboxMsg.UserID // Assuming UserID is a string on OutboxMessage (it is in core_sms/domain)

        selectedProvider, routeErr := s.router.SelectProvider(ctx, outboxMsg.Recipient, userIDStr)
        if routeErr != nil {
            s.logger.ErrorContext(ctx, "Error selecting provider from router", "error", routeErr, "outbox_message_id", outboxMsg.ID)
            errMsg := fmt.Sprintf("Routing error: %v", routeErr)
            sentAt := time.Now()
            // Consider a specific status for routing failure if different from general config error
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, domain.MessageStatusFailedConfiguration, nil, nil, sentAt, &errMsg); errUpdate != nil {
                s.logger.ErrorContext(ctx, "Failed to update outbox after routing error", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf(errMsg) // Stop processing this job due to routing error
        }

        if selectedProvider == nil { // No specific route matched, fallback to default provider
            s.logger.InfoContext(ctx, "No specific route matched, using default provider", "default_provider", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
            var ok bool
            selectedProvider, ok = s.providers[s.defaultProviderName]
            if !ok || selectedProvider == nil { // Check selectedProvider for nil explicitly
                s.logger.ErrorContext(ctx, "Default SMS provider not found or not configured", "provider_name", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
                errMsg := fmt.Sprintf("Default SMS provider '%s' not found/configured", s.defaultProviderName)
                sentAt := time.Now()
                if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, domain.MessageStatusFailedConfiguration, nil, nil, sentAt, &errMsg); errUpdate != nil {
                    s.logger.ErrorContext(ctx, "Failed to update outbox after default provider configuration error", "error", errUpdate, "id", outboxMsg.ID)
                }
                return fmt.Errorf(errMsg) // Stop processing this job
            }
        }

        s.logger.InfoContext(ctx, "Sending SMS via provider", "provider", selectedProvider.GetName(), "recipient", outboxMsg.Recipient, "outbox_message_id", outboxMsg.ID)

        providerCtx, providerCancel := context.WithTimeout(ctx, 30*time.Second) // Timeout for provider call
        defer providerCancel()
        providerResponse, sendErr := selectedProvider.Send(providerCtx, sendDetails)
        sentAt := time.Now()

        // 4. Update OutboxMessage based on provider response
        if sendErr != nil {
            s.logger.ErrorContext(ctx, "Failed to send SMS via provider", "error", sendErr, "provider", selectedProvider.GetName(), "outbox_message_id", outboxMsg.ID)
            errMsg := sendErr.Error()
            providerStatusStr := ""
            if providerResponse != nil { // Provider might return a response even on error
                providerStatusStr = providerResponse.ProviderStatus
            }
            // Use existing tx for this update
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, domain.MessageStatusFailedProviderSubmission, nil, &providerStatusStr, sentAt, &errMsg); errUpdate != nil {
                s.logger.ErrorContext(ctx, "Failed to update outbox after provider send failure", "error", errUpdate, "id", outboxMsg.ID)
            }
            // Return sendErr to ensure the transaction is rolled back if this is considered a true failure for the job.
            // Or, if the update was successful, and this is just a provider failure to be logged, return nil for tx.
            // For now, let's assume provider failure means the job processing failed overall.
            return fmt.Errorf("provider send error for %s: %w", selectedProvider.GetName(), sendErr)
        }

        s.logger.InfoContext(ctx, "SMS submitted to provider successfully", "provider", selectedProvider.GetName(), "provider_msg_id", providerResponse.ProviderMessageID, "outbox_message_id", outboxMsg.ID)
        // Use existing tx for this update
        if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, domain.MessageStatusSentToProvider, &providerResponse.ProviderMessageID, &providerResponse.ProviderStatus, sentAt, nil); errUpdate != nil {
            s.logger.ErrorContext(ctx, "Failed to update outbox after successful provider submission", "error", errUpdate, "id", outboxMsg.ID)
            return fmt.Errorf("db update (post send info) failed: %w", errUpdate)
        }
        return nil // Transaction successful
    })

    return txErr
}


// StopConsumingJobs unsubscribes from NATS.
func (s *SMSSendingAppService) StopConsumingJobs() {
	if s.natsSub != nil && s.natsSub.IsValid() {
		s.logger.Info("Unsubscribing from NATS job subject", "subject", s.natsSub.Subject)
		if err := s.natsSub.Unsubscribe(); err != nil {
			s.logger.Error("Failed to unsubscribe from NATS", "error", err, "subject", s.natsSub.Subject)
		}
		// Drain may not be needed if Unsubscribe is sufficient and connection is closed by main.
        // if err := s.natsSub.Drain(); err != nil {
        //      s.logger.Error("Failed to drain NATS subscription", "error", err, "subject", s.natsSub.Subject)
        // }
	}
}
