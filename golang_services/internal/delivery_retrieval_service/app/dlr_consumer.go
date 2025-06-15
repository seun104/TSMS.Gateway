package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/domain" // Corrected
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker" // Corrected
)

// DLREvent bundles the provider name and the DLR data from the NATS message.
type DLREvent struct {
	ProviderName string
	RequestData  domain.ProviderDLRCallbackRequest
}

// DLRConsumer is responsible for consuming DLR messages from NATS.
type DLRConsumer struct {
	natsClient *messagebroker.NATSClient
	logger     *slog.Logger
	outputChan chan<- DLREvent // Channel to send deserialized DLR events for processing
}

// NewDLRConsumer creates a new DLRConsumer.
func NewDLRConsumer(natsClient *messagebroker.NATSClient, logger *slog.Logger, outputChan chan<- DLREvent) *DLRConsumer {
	return &DLRConsumer{
		natsClient: natsClient,
		logger:     logger,
		outputChan: outputChan,
	}
}

// StartConsuming subscribes to the given NATS subject for DLR messages.
// It processes messages by deserializing them and sending them to the outputChan.
// This method is blocking and designed to be run in a goroutine.
// It respects context cancellation for graceful shutdown.
func (c *DLRConsumer) StartConsuming(ctx context.Context, subject string, queueGroup string) error {
	msgHandler := func(msg *nats.Msg) { // nats.Msg does not carry a context directly, use the consumer's ctx
		// Create a logger for this specific message, including NATS subject
		msgLogger := c.logger.With("nats_subject", msg.Subject)
		msgLogger.InfoContext(ctx, "Received NATS DLR message", "data_len", len(msg.Data))

		subjectParts := strings.Split(msg.Subject, ".")
		if len(subjectParts) < 3 || subjectParts[0] != "dlr" || subjectParts[1] != "raw" {
			msgLogger.ErrorContext(ctx, "Invalid NATS subject format for DLR")
			return
		}
		providerName := subjectParts[2]
		if providerName == "" || providerName == "*" || providerName == ">" {
			msgLogger.ErrorContext(ctx, "Could not determine provider name from DLR subject or it's a wildcard")
			return
		}

		// Add provider_name to logger context for subsequent logs for this message
		msgLogger = msgLogger.With("provider_name", providerName)

		var dlrData domain.ProviderDLRCallbackRequest
		if err := json.Unmarshal(msg.Data, &dlrData); err != nil {
			msgLogger.ErrorContext(ctx, "Failed to deserialize NATS DLR message data",
				"error", err, "data", string(msg.Data))
			return
		}

		// Add more specific DLR identifiers to logger
		msgLogger = msgLogger.With(
			"provider_message_id", dlrData.ProviderMessageID,
			"internal_message_id", dlrData.MessageID, // Assuming this is our internal ID if available
		)

		msgLogger.InfoContext(ctx, "Successfully deserialized DLR data", "status", dlrData.Status)

		event := DLREvent{
			ProviderName: providerName,
			RequestData:  dlrData,
		}

		// Use a context for the select, possibly derived from the main consumer context
		// to allow timeout for sending to channel if it's blocked.
		sendCtx, cancelSend := context.WithTimeout(ctx, 5*time.Second) // Example timeout
		defer cancelSend()

		select {
		case c.outputChan <- event:
			msgLogger.DebugContext(sendCtx, "Sent deserialized DLR event to processing channel")
		case <-sendCtx.Done(): // If timeout occurs sending to channel
			msgLogger.ErrorContext(sendCtx, "Timed out sending DLR event to processing channel", "error", sendCtx.Err())
			return
		case <-ctx.Done(): // If main consumer context is cancelled
			msgLogger.InfoContext(ctx, "Context cancelled, not sending DLR event to processing channel", "error", ctx.Err())
			return
		}
	}

	c.logger.InfoContext(ctx, "Starting NATS DLR subscription", "subject", subject, "queue_group", queueGroup)
	// Assuming SubscribeToSubjectWithQueue is part of NATSClient interface and handles context for shutdown
	err := c.natsClient.SubscribeToSubjectWithQueue(ctx, subject, queueGroup, msgHandler)
	if err != nil {
		c.logger.ErrorContext(ctx, "NATS DLR subscription failed", "error", err, "subject", subject)
		return err
	}

	// This log might be reached if SubscribeToSubjectWithQueue is non-blocking and returns nil on successful setup,
    // or after it returns due to context cancellation if it's blocking.
	// If blocking, it will log "NATS DLR subscription ended" when ctx is cancelled.
	<-ctx.Done() // Wait for context cancellation to signal shutdown
	c.logger.InfoContext(ctx, "NATS DLR subscription processing loop ended due to context cancellation.", "subject", subject)
	return nil
}

// Added time import for context.WithTimeout
import "time"
