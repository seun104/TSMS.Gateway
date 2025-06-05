package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/your-repo/project/internal/delivery_retrieval_service/domain"
	"github.com/your-repo/project/internal/platform/messagebroker"
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
	msgHandler := func(msg *nats.Msg) {
		c.logger.InfoContext(ctx, "Received NATS DLR message", "subject", msg.Subject, "data_len", len(msg.Data))

		subjectParts := strings.Split(msg.Subject, ".")
		if len(subjectParts) < 3 || subjectParts[0] != "dlr" || subjectParts[1] != "raw" {
			c.logger.ErrorContext(ctx, "Invalid NATS subject format for DLR", "subject", msg.Subject)
			return
		}
		providerName := subjectParts[2]
		if providerName == "" || providerName == "*" || providerName == ">" {
			c.logger.ErrorContext(ctx, "Could not determine provider name from DLR subject or it's a wildcard", "subject", msg.Subject)
			return
		}

		var dlrData domain.ProviderDLRCallbackRequest
		if err := json.Unmarshal(msg.Data, &dlrData); err != nil {
			c.logger.ErrorContext(ctx, "Failed to deserialize NATS DLR message data",
				"error", err, "subject", msg.Subject, "data", string(msg.Data))
			// Consider sending to a dead-letter queue.
			return
		}

		c.logger.InfoContext(ctx, "Successfully deserialized DLR data",
			"provider_name", providerName,
			"message_id", dlrData.MessageID,
			"status", dlrData.Status,
		)

		event := DLREvent{
			ProviderName: providerName,
			RequestData:  dlrData,
		}

		select {
		case c.outputChan <- event:
			c.logger.DebugContext(ctx, "Sent deserialized DLR event to processing channel", "provider_name", providerName, "message_id", dlrData.MessageID)
		case <-ctx.Done():
			c.logger.InfoContext(ctx, "Context cancelled, not sending DLR event to processing channel",
				"provider_name", providerName, "message_id", dlrData.MessageID, "error", ctx.Err())
			return
		}
	}

	c.logger.InfoContext(ctx, "Starting NATS DLR subscription", "subject", subject, "queue_group", queueGroup)
	err := c.natsClient.SubscribeToSubjectWithQueue(ctx, subject, queueGroup, msgHandler)
	if err != nil {
		c.logger.ErrorContext(ctx, "NATS DLR subscription failed", "error", err, "subject", subject)
		return err
	}

	c.logger.InfoContext(ctx, "NATS DLR subscription ended.", "subject", subject)
	return nil
}
