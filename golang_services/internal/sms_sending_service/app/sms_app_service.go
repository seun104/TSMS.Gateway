package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aradsms/golang_services/api/proto/billingservice" // gRPC client for billing
	coreSmsDomain "github.com/aradsms/golang_services/internal/core_sms/domain" // Alias for clarity
	"github.com/aradsms/golang_services/internal/platform/messagebroker" // NATS
	blacklistDomain "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain" // New blacklist domain
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository" // For OutboxRepository
	"github.com/google/uuid" // For parsing UserID string to UUID
	"github.com/nats-io/nats.go"
    "github.com/jackc/pgx/v5/pgxpool" // For DB transactions
    "github.com/jackc/pgx/v5"       // For pgx.Tx
	"strings"                       // For content filtering
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
	router              *Router
	blacklistRepo       blacklistDomain.BlacklistRepository
	filterWordRepo      blacklistDomain.FilterWordRepository // Added filter word repository
	natsSub             *nats.Subscription
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
	router *Router,
	blacklistRepo blacklistDomain.BlacklistRepository,
	filterWordRepo blacklistDomain.FilterWordRepository, // Added
) *SMSSendingAppService {
	return &SMSSendingAppService{
		outboxRepo:          outboxRepo,
		providers:           providers,
		defaultProviderName: defaultProviderName,
		billingClient:       billingClient,
		natsClient:          natsClient,
		dbPool:              dbPool,
		logger:              logger.With("service", "sms_sending_app"),
		router:              router,
		blacklistRepo:       blacklistRepo,
		filterWordRepo:      filterWordRepo, // Added
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

    var outboxMsg *coreSmsDomain.OutboxMessage // Use alias for core SMS domain
    var err error

    // Start a database transaction
    txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
        outboxMsg, err = s.outboxRepo.GetByID(ctx, tx, job.OutboxMessageID)
        if err != nil {
            s.logger.ErrorContext(ctx, "Failed to get outbox message", "error", err, "id", job.OutboxMessageID)
            return fmt.Errorf("outbox message not found: %w", err)
        }

        if outboxMsg.Status != coreSmsDomain.MessageStatusQueued {
            s.logger.WarnContext(ctx, "SMS job already processed or in invalid state", "id", job.OutboxMessageID, "status", outboxMsg.Status)
            return nil
        }

		// ** BLACKLIST CHECK START **
		var msgUserID uuid.NullUUID
		if outboxMsg.UserID != "" {
			parsedUUID, parseErr := uuid.Parse(outboxMsg.UserID)
			if parseErr == nil {
				msgUserID = uuid.NullUUID{UUID: parsedUUID, Valid: true}
			} else {
				s.logger.WarnContext(ctx, "Failed to parse UserID from outbox message for blacklist check", "user_id_str", outboxMsg.UserID, "message_id", outboxMsg.ID, "error", parseErr)
			}
		}

		isBlacklisted, blReason, blErr := s.blacklistRepo.IsBlacklisted(ctx, outboxMsg.Recipient, msgUserID)
		if blErr != nil {
			s.logger.ErrorContext(ctx, "Error checking blacklist", "error", blErr, "recipient", outboxMsg.Recipient, "message_id", outboxMsg.ID)
			// Decide if this is a hard fail or soft fail. For now, log and continue (provider will likely fail if number is truly invalid).
            // For a production system, this might be a reason to fail the transaction.
            // Let's change to fail the job if blacklist check itself errors out.
            errMsg := fmt.Sprintf("Blacklist check failed: %v", blErr)
            now := time.Now().UTC()
            // Assuming UpdatePostSendInfo can handle general failures before sending.
            // A more specific status like MessageStatusSystemError might be better.
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailed, nil, nil, now, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox after blacklist check error", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf(errMsg) // Fail the transaction and job processing
		}

		if isBlacklisted {
			s.logger.InfoContext(ctx, "Recipient is blacklisted", "recipient", outboxMsg.Recipient, "reason", blReason, "message_id", outboxMsg.ID)
			errMsg := "Recipient blacklisted"
			if blReason != "" {
				errMsg = fmt.Sprintf("Recipient blacklisted: %s", blReason)
			}
			now := time.Now().UTC()
            // Using UpdatePostSendInfo to set a final status.
            // ProviderMessageID and ProviderStatus would be nil/empty.
			if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusRejected, nil, nil, now, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox for blacklisted message", "error", errUpdate, "id", outboxMsg.ID)
            }
			return nil
		}
		// ** BLACKLIST CHECK END **

		// ** CONTENT FILTERING START **
		activeFilterWords, fwErr := s.filterWordRepo.GetActiveFilterWords(ctx)
		if fwErr != nil {
			s.logger.ErrorContext(ctx, "Error fetching active filter words", "error", fwErr, "message_id", outboxMsg.ID)
            errMsg := fmt.Sprintf("Filter word check failed: %v", fwErr)
            now := time.Now().UTC()
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailed, nil, nil, now, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox after filter word check error", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf(errMsg) // Fail the transaction
		}

		if len(activeFilterWords) > 0 {
			normalizedContent := strings.ToLower(outboxMsg.Content)
			var matchedFilterWord string
			for _, filterWord := range activeFilterWords {
				if strings.Contains(normalizedContent, strings.ToLower(filterWord)) {
					matchedFilterWord = filterWord
					break
				}
			}

			if matchedFilterWord != "" {
				s.logger.InfoContext(ctx, "Message content filtered",
					"recipient", outboxMsg.Recipient,
					"matched_word", matchedFilterWord,
					"message_id", outboxMsg.ID)

				errMsg := fmt.Sprintf("Message content rejected due to filter word: %s", matchedFilterWord)
				now := time.Now().UTC()
				if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusRejected, nil, nil, now, &errMsg); errUpdate != nil {
					s.logger.ErrorContext(ctx, "Failed to update outbox for filtered message", "error", errUpdate, "id", outboxMsg.ID)
				}
				return nil // Message rejected due to filter, commit transaction with rejected state.
			}
		}
		// ** CONTENT FILTERING END **

        // Update status to "processing" - moved after all checks
        processingTime := time.Now().UTC()
        if errStatusUpdate := s.outboxRepo.UpdateStatus(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusProcessing, &processingTime, nil, nil); errStatusUpdate != nil {
             s.logger.ErrorContext(ctx, "Failed to update outbox status to processing", "error", errStatusUpdate, "id", outboxMsg.ID)
             return fmt.Errorf("db update (processing) failed: %w", errStatusUpdate)
        }
        outboxMsg.Status = coreSmsDomain.MessageStatusProcessing
        outboxMsg.ProcessedAt = &processingTime


        // 2. Check & Deduct Credit via Billing Service (No changes here from original logic)
        amountToDeduct := 1.0 // Placeholder: 1 unit of credit per SMS
        deductReq := &billingservice.DeductCreditRequest{
            UserId:           outboxMsg.UserID,
            // AmountToDeduct:   amountToDeduct, // This field is deprecated in proto
            SegmentsToCharge: int32(outboxMsg.Segments), // Assuming this is the new field and maps to segments
            // TransactionType:  string(coreSmsDomain.TransactionTypeSMSCharge), // Type is determined by billing service now
            Description:      fmt.Sprintf("SMS charge for message to %s (msgID: %s)", outboxMsg.Recipient, outboxMsg.ID),
            ReferenceId:      outboxMsg.ID, // Use outboxMsg.ID as reference for billing transaction
        }
        s.logger.InfoContext(ctx, "Calling billing service to deduct credit", "userID", outboxMsg.UserID, "segments", outboxMsg.Segments)

        billingCtx, billingCancel := context.WithTimeout(ctx, 30*time.Second) // Timeout for gRPC call
        defer billingCancel()
        _, errBilling := s.billingClient.DeductCredit(billingCtx, deductReq)
        if errBilling != nil {
            s.logger.ErrorContext(ctx, "Failed to deduct credit via billing service", "error", errBilling, "userID", outboxMsg.UserID)
            errMsg := fmt.Sprintf("Billing error: %v", errBilling)
            now := time.Now().UTC()
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedProviderSubmission, nil, nil, now, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox after billing failure", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf("credit deduction failed: %w", errBilling)
        }
        s.logger.InfoContext(ctx, "Credit deducted successfully", "userID", outboxMsg.UserID)

        // 3. Send SMS via Provider (logic for selecting provider using router is already here from previous step)
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
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedConfiguration, nil, nil, sentAt, &errMsg); errUpdate != nil { // Use coreSmsDomain
                s.logger.ErrorContext(ctx, "Failed to update outbox after routing error", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf(errMsg) // Stop processing this job due to routing error
        }

        if selectedProvider == nil { // No specific route matched, fallback to default provider
            s.logger.InfoContext(ctx, "No specific route matched, using default provider", "default_provider", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
            var ok bool
            selectedProvider, ok = s.providers[s.defaultProviderName]
            if !ok || selectedProvider == nil {
                s.logger.ErrorContext(ctx, "Default SMS provider not found or not configured", "provider_name", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
                errMsg := fmt.Sprintf("Default SMS provider '%s' not found/configured", s.defaultProviderName)
                now := time.Now().UTC()
                if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedConfiguration, nil, nil, now, &errMsg); errUpdate != nil {
                    s.logger.ErrorContext(ctx, "Failed to update outbox after default provider configuration error", "error", errUpdate, "id", outboxMsg.ID)
                }
                return fmt.Errorf(errMsg)
            }
        }

        s.logger.InfoContext(ctx, "Sending SMS via provider", "provider", selectedProvider.GetName(), "recipient", outboxMsg.Recipient, "outbox_message_id", outboxMsg.ID)

        providerCtx, providerCancel := context.WithTimeout(ctx, 30*time.Second)
        defer providerCancel()
        providerResponse, sendErr := selectedProvider.Send(providerCtx, sendDetails)
        now := time.Now().UTC()

        if sendErr != nil {
            s.logger.ErrorContext(ctx, "Failed to send SMS via provider", "error", sendErr, "provider", selectedProvider.GetName(), "outbox_message_id", outboxMsg.ID)
            errMsg := sendErr.Error()
            providerStatusStr := ""
            if providerResponse != nil {
                providerStatusStr = providerResponse.ProviderStatus
            }
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedProviderSubmission, nil, &providerStatusStr, now, &errMsg); errUpdate != nil {
                s.logger.ErrorContext(ctx, "Failed to update outbox after provider send failure", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf("provider send error for %s: %w", selectedProvider.GetName(), sendErr)
        }

        s.logger.InfoContext(ctx, "SMS submitted to provider successfully", "provider", selectedProvider.GetName(), "provider_msg_id", providerResponse.ProviderMessageID, "outbox_message_id", outboxMsg.ID)
        if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusSentToProvider, &providerResponse.ProviderMessageID, &providerResponse.ProviderStatus, now, nil); errUpdate != nil {
            s.logger.ErrorContext(ctx, "Failed to update outbox after successful provider submission", "error", errUpdate, "id", outboxMsg.ID)
            return fmt.Errorf("db update (post send info) failed: %w", errUpdate)
        }
        return nil
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
