package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice" // Corrected gRPC client for billing
	coreSmsDomain "github.com/AradIT/aradsms/golang_services/internal/core_sms/domain" // Corrected Alias for clarity
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker" // Corrected NATS
	blacklistDomain "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/provider"     // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/repository" // Corrected For OutboxRepository
	"github.com/google/uuid"                                                            // For parsing UserID string to UUID
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
		natsSMSJobsReceivedCounter.WithLabelValues(msg.Subject).Inc() // Increment NATS counter
		s.logger.Info("Received NATS job", "subject", msg.Subject, "data_len", len(msg.Data))

		var job NATSJobPayload
		if err := json.Unmarshal(msg.Data, &job); err != nil {
			s.logger.Error("Failed to unmarshal NATS job payload", "error", err, "data", string(msg.Data))
			// Nack? Dead-letter queue? For now, just log.
			// No specific metric for unmarshal error here, but processSMSJob will fail and be counted.
			return
		}

		// Process the job within a new context to handle potential timeouts per job
		jobCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Example timeout
		defer cancel()

		// processSMSJob will handle its own metrics for processing duration and status
		if err := s.processSMSJob(jobCtx, job); err != nil {
			s.logger.Error("Failed to process SMS job (after call to processSMSJob)", "error", err, "outbox_message_id", job.OutboxMessageID)
			// The error from processSMSJob (and its metrics) are already handled within processSMSJob.
			// This log is for any overarching issues if processSMSJob itself returns an error that needs logging here.
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
	var chosenProviderName string = "unknown" // Default if provider selection fails early
	var jobStatus string = "success"         // Assume success, will be changed on error

	// Defer the main job processing counter update
	defer func() {
		// This will be executed when processSMSJob returns.
		// chosenProviderName and jobStatus should be set appropriately by then.
		smsSendingProcessedCounter.WithLabelValues(chosenProviderName, jobStatus).Inc()
	}()

	// Start timer for overall job processing duration (excluding NATS unmarshal)
	// Note: chosenProviderName might not be known until router.SelectProvider.
	// We can start the timer here and update labels later, or use a generic label first.
	// For simplicity, we'll use the determined provider name. If it errors before that, it's "unknown".
	// A better way would be to have the timer observe after chosenProviderName is set.
	// Let's move timer start to after provider selection, or pass chosenProviderName to defer.

	s.logger.InfoContext(ctx, "Processing SMS job", "outbox_message_id", job.OutboxMessageID)

    var outboxMsg *coreSmsDomain.OutboxMessage
    var err error

    // Start a database transaction
    txErr := pgx.BeginFunc(ctx, s.dbPool, func(tx pgx.Tx) error {
		processingTimer := prometheus.NewTimer(nil) // Placeholder, will be set with labels later

        outboxMsg, err = s.outboxRepo.GetByID(ctx, tx, job.OutboxMessageID)
        if err != nil {
			jobStatus = "error_db_fetch_outbox"
            s.logger.ErrorContext(ctx, "Failed to get outbox message", "error", err, "id", job.OutboxMessageID)
            return fmt.Errorf("outbox message not found: %w", err)
        }
		// Now we have outboxMsg, we can try to determine provider for duration histogram early
		// but actual provider selection happens later. This is a bit tricky for the main duration timer.
		// Let's assume a temporary provider name or set it after selection.

        if outboxMsg.Status != coreSmsDomain.MessageStatusQueued {
			jobStatus = "error_already_processed" // Or "warn_already_processed" if not treated as an error
            s.logger.WarnContext(ctx, "SMS job already processed or in invalid state", "id", job.OutboxMessageID, "status", outboxMsg.Status)
            return nil // Not an error for the transaction itself, but job won't be processed further.
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
            // A more specific status like MessageStatusSystemError might be better.
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailed, nil, nil, now, &errMsg); errUpdate != nil {
                 s.logger.ErrorContext(ctx, "Failed to update outbox after blacklist check error", "error", errUpdate, "id", outboxMsg.ID)
            }
			jobStatus = "error_blacklist_check"
            return fmt.Errorf(errMsg) // Fail the transaction and job processing
		}

		if isBlacklisted {
			jobStatus = "rejected_blacklist"
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
			// No error returned to txErr, but jobStatus is set for metrics
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
			jobStatus = "error_filterword_check"
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
				jobStatus = "rejected_filterword"
				return nil // Message rejected due to filter, commit transaction with rejected state.
			}
		}
		// ** CONTENT FILTERING END **

        // Update status to "processing" - moved after all checks
        processingTime := time.Now().UTC()
        if errStatusUpdate := s.outboxRepo.UpdateStatus(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusProcessing, &processingTime, nil, nil); errStatusUpdate != nil {
			jobStatus = "error_db_update_processing"
            s.logger.ErrorContext(ctx, "Failed to update outbox status to processing", "error", errStatusUpdate, "id", outboxMsg.ID)
            return fmt.Errorf("db update (processing) failed: %w", errStatusUpdate)
        }
        outboxMsg.Status = coreSmsDomain.MessageStatusProcessing // Keep local model in sync
        outboxMsg.ProcessedAt = &processingTime // Keep local model in sync


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

        billingCtx, billingCancel := context.WithTimeout(ctx, 30*time.Second)
        defer billingCancel()

		billingTimer := prometheus.NewTimer(billingServiceRequestDurationHist.WithLabelValues("DeductUserCreditForSMS"))
        _, errBilling := s.billingClient.DeductCredit(billingCtx, deductReq)
		billingTimer.ObserveDuration()

        if errBilling != nil {
			jobStatus = "error_billing_deduct"
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
		chosenProviderName = s.defaultProviderName // Default, will be updated if specific provider is chosen
        if routeErr != nil {
			jobStatus = "error_routing"
            s.logger.ErrorContext(ctx, "Error selecting provider from router", "error", routeErr, "outbox_message_id", outboxMsg.ID)
            errMsg := fmt.Sprintf("Routing error: %v", routeErr)
            sentAt := time.Now()
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedConfiguration, nil, nil, sentAt, &errMsg); errUpdate != nil {
                s.logger.ErrorContext(ctx, "Failed to update outbox after routing error", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf(errMsg)
        }

        if selectedProvider == nil {
            s.logger.InfoContext(ctx, "No specific route matched, using default provider", "default_provider", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
            var ok bool
            selectedProvider, ok = s.providers[s.defaultProviderName]
            if !ok || selectedProvider == nil {
				jobStatus = "error_provider_config"
                s.logger.ErrorContext(ctx, "Default SMS provider not found or not configured", "provider_name", s.defaultProviderName, "outbox_message_id", outboxMsg.ID)
                errMsg := fmt.Sprintf("Default SMS provider '%s' not found/configured", s.defaultProviderName)
                now := time.Now().UTC()
                if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedConfiguration, nil, nil, now, &errMsg); errUpdate != nil {
                    s.logger.ErrorContext(ctx, "Failed to update outbox after default provider configuration error", "error", errUpdate, "id", outboxMsg.ID)
                }
                return fmt.Errorf(errMsg)
            }
        }
		chosenProviderName = selectedProvider.GetName() // Set for metrics
		processingTimer = prometheus.NewTimer(smsSendingProcessingDurationHist.WithLabelValues(chosenProviderName)) // Re-init timer with correct label
		defer processingTimer.ObserveDuration() // This will observe duration for the rest of the function scope

        s.logger.InfoContext(ctx, "Sending SMS via provider", "provider", selectedProvider.GetName(), "recipient", outboxMsg.Recipient, "outbox_message_id", outboxMsg.ID)

        providerCtx, providerCancel := context.WithTimeout(ctx, 30*time.Second)
        defer providerCancel()
        providerResponse, sendErr := selectedProvider.Send(providerCtx, sendDetails)
        now := time.Now().UTC()

        if sendErr != nil {
			jobStatus = "error_provider_send"
            s.logger.ErrorContext(ctx, "Failed to send SMS via provider", "error", sendErr, "provider", chosenProviderName, "outbox_message_id", outboxMsg.ID)
            errMsg := sendErr.Error()
            providerStatusStr := ""
            if providerResponse != nil {
                providerStatusStr = providerResponse.ProviderStatus
            }
            if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusFailedProviderSubmission, nil, &providerStatusStr, now, &errMsg); errUpdate != nil {
                s.logger.ErrorContext(ctx, "Failed to update outbox after provider send failure", "error", errUpdate, "id", outboxMsg.ID)
            }
            return fmt.Errorf("provider send error for %s: %w", chosenProviderName, sendErr)
        }

		// Increment segments sent counter
		// Assuming outboxMsg.Segments is correctly calculated before this point (e.g., during initial creation or update)
		if outboxMsg.Segments > 0 {
			smsSegmentsSentCounter.WithLabelValues(chosenProviderName).Add(float64(outboxMsg.Segments))
		}


        s.logger.InfoContext(ctx, "SMS submitted to provider successfully", "provider", chosenProviderName, "provider_msg_id", providerResponse.ProviderMessageID, "outbox_message_id", outboxMsg.ID)
        if errUpdate := s.outboxRepo.UpdatePostSendInfo(ctx, tx, outboxMsg.ID, coreSmsDomain.MessageStatusSentToProvider, &providerResponse.ProviderMessageID, &providerResponse.ProviderStatus, now, nil); errUpdate != nil {
			jobStatus = "error_db_update_sent"
            s.logger.ErrorContext(ctx, "Failed to update outbox after successful provider submission", "error", errUpdate, "id", outboxMsg.ID)
            return fmt.Errorf("db update (post send info) failed: %w", errUpdate)
        }
        // jobStatus remains "success"
        return nil
    })

    // If txErr is not nil, it means the transaction failed.
    // The jobStatus inside the txFunc might have been set to a specific error.
    // If txErr is nil, the transaction committed, and jobStatus reflects the outcome within the transaction.
    if txErr != nil && jobStatus == "success" {
        // If the transaction itself failed (e.g., commit error, or an error returned from txFunc that wasn't a business logic error)
        // and we haven't set a more specific jobStatus, mark it as a generic processing error.
        jobStatus = "error_processing_transaction"
    }
    // The deferred smsSendingProcessedCounter.Inc() will use the final jobStatus.
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
