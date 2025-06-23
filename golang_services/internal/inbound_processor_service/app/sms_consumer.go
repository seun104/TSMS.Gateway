package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings" // To extract provider name from subject
	"time"    // For select with timeout on channel send

	"github.com/nats-io/nats.go"
	"github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/domain" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"           // Corrected path
)

// SMSConsumer is responsible for consuming raw incoming SMS messages from NATS
// and forwarding them for processing.
type SMSConsumer struct {
	natsClient *messagebroker.NATSClient
	logger     *slog.Logger
	// msgChan is used to send the deserialized message (and provider name) to the processing stage.
	// Using a struct to bundle the DTO and provider name.
	outputChan chan<- InboundSMSEvent
}

// InboundSMSEvent holds the deserialized message and the provider name from the NATS subject.
type InboundSMSEvent struct {
	ProviderName string
	Data         domain.ProviderIncomingSMSRequest
}

// NewSMSConsumer creates a new SMSConsumer instance.
// outputChan is a channel where successfully deserialized messages and their provider are sent.
func NewSMSConsumer(natsClient *messagebroker.NATSClient, logger *slog.Logger, outputChan chan<- InboundSMSEvent) *SMSConsumer {
	return &SMSConsumer{
		natsClient: natsClient,
		logger:     logger,
		outputChan: outputChan,
	}
}

// StartConsuming subscribes to the given NATS subject and processes messages.
// It listens for messages on 'subject' (e.g., "sms.incoming.raw.*") using 'queueGroup'.
// This method will block until the context is cancelled or an unrecoverable error occurs during subscription.
func (c *SMSConsumer) StartConsuming(ctx context.Context, subject string, queueGroup string) error {
	msgHandler := func(msg *nats.Msg) {
		// Increment NATS messages received counter using the subscribed subject pattern
		natsInboundSMSReceivedCounter.WithLabelValues(subject).Inc()

		c.logger.InfoContext(ctx, "Received NATS message", "subject", msg.Subject, "reply", msg.Reply, "data_len", len(msg.Data))

		// Extract provider_name from subject. Example: "sms.incoming.raw.providerX" -> "providerX"
		subjectParts := strings.Split(msg.Subject, ".")
		if len(subjectParts) < 4 || subjectParts[0] != "sms" || subjectParts[1] != "incoming" || subjectParts[2] != "raw" {
			c.logger.ErrorContext(ctx, "Invalid NATS subject format for incoming SMS", "subject", msg.Subject)
			// Acknowledge message if it's JetStream, or simply return if core NATS and not expecting ack
			// For simplicity, assuming core NATS or auto-ack for now. If JetStream, msg.Ack() or msg.Nak() would be needed.
			return
		}
		providerName := subjectParts[3]
		if providerName == "" || providerName == "*" || providerName == ">" {
			c.logger.ErrorContext(ctx, "Could not determine provider name from subject or it's a wildcard", "subject", msg.Subject)
			return
		}


		var req domain.ProviderIncomingSMSRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			c.logger.ErrorContext(ctx, "Failed to deserialize NATS message data into ProviderIncomingSMSRequest",
				"error", err, "subject", msg.Subject, "data", string(msg.Data))
			// Consider sending to a dead-letter queue or logging persistently if this happens often.
			return
		}

		c.logger.InfoContext(ctx, "Successfully deserialized incoming SMS",
			"provider_name", providerName,
			"from", req.From, "to", req.To, "message_id", req.MessageID,
		)

		// Send the deserialized message and provider name to the processing channel
		// Use a select with context to avoid blocking indefinitely if the channel is full and context is cancelled.
		// Use a context for the select, possibly derived from the main consumer context
		// to allow timeout for sending to channel if it's blocked.
		sendCtx, cancelSend := context.WithTimeout(ctx, 5*time.Second) // Example timeout
		defer cancelSend()

		select {
		case c.outputChan <- InboundSMSEvent{ProviderName: providerName, Data: req}:
			c.logger.DebugContext(sendCtx, "Sent deserialized SMS to processing channel", "provider_name", providerName, "message_id", req.MessageID)
		case <-sendCtx.Done(): // If timeout occurs sending to channel
			c.logger.ErrorContext(sendCtx, "Timed out sending deserialized SMS to processing channel", "error", sendCtx.Err(), "provider_name", providerName, "message_id", req.MessageID)
			return
		case <-ctx.Done(): // If main consumer context is cancelled
			c.logger.InfoContext(ctx, "Context cancelled, not sending SMS to processing channel", "provider_name", providerName, "message_id", req.MessageID)
			return
		}
	}

	// The SubscribeWithQueue method in the platform/messagebroker NATSClient should handle
	// running the handler in a goroutine and managing context for shutdown.
	// It is expected to be a blocking call that returns when the subscription ends (e.g., context cancelled).
	c.logger.InfoContext(ctx, "Starting NATS subscription", "subject", subject, "queue_group", queueGroup)
	err := c.natsClient.SubscribeToSubjectWithQueue(ctx, subject, queueGroup, msgHandler)
	if err != nil {
		c.logger.ErrorContext(ctx, "NATS subscription failed", "error", err, "subject", subject)
		return err // Propagate error to allow main/caller to handle (e.g. retry, shutdown)
	}

	c.logger.InfoContext(ctx, "NATS subscription ended.", "subject", subject)
	return nil // Normal termination (e.g. context cancelled)
}
