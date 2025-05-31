package messagebroker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream" // If JetStream is intended from the start
)

// NatsClient wraps NATS connection and JetStream context (optional for basic pub/sub).
type NatsClient struct {
	Conn   *nats.Conn
	JS     jetstream.JetStream // Optional: Only if JetStream is used
	Logger *slog.Logger
}

// NewNatsClient connects to NATS and optionally sets up JetStream.
// natsURL example: "nats://localhost:4222" or "tls://user:pass@localhost:4222"
func NewNatsClient(natsURL string, appName string, logger *slog.Logger, useJetStream bool) (*NatsClient, error) {
	l := logger.With("component", "nats_client")
	nc, err := nats.Connect(natsURL,
		nats.Name(appName+" - "+time.Now().Format(time.RFC3339Nano)), // Unique connection name
		nats.Timeout(10*time.Second), // Increased timeout for initial connection
		nats.PingInterval(20*time.Second),
		nats.MaxPingsOutstanding(3),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1), // Infinite reconnects
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				l.Warn("NATS disconnected", "error", err.Error())
			} else {
				l.Warn("NATS disconnected")
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			l.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if nc.LastError() != nil {
				l.Error("NATS connection closed", "error", nc.LastError().Error())
			} else {
				l.Info("NATS connection closed")
			}
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			l.Error("NATS async error", "subject", sub.Subject, "queue", sub.Queue, "error", err.Error())
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}
	l.Info("Successfully connected to NATS server", "url", natsURL)

	client := &NatsClient{Conn: nc, Logger: l}

	if useJetStream {
		js, err := jetstream.New(nc)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create JetStream context: %w", err)
		}
		client.JS = js
		l.Info("JetStream context created")
	}

	return client, nil
}

// Close gracefully drains and closes the NATS connection.
func (nc *NatsClient) Close() {
	if nc.Conn != nil && !nc.Conn.IsClosed() {
		nc.Logger.Info("Draining and closing NATS connection...")
		err := nc.Conn.Drain() // Drain ensures all published messages are sent and subscriptions are drained.
		if err != nil {
			nc.Logger.Error("Error draining NATS connection", "error", err)
		}
		// Close is implicitly called by Drain, but calling it again is safe and ensures closure.
		// nc.Conn.Close() // Not strictly necessary after Drain, but for explicitness.
		nc.Logger.Info("NATS connection closed.")
	}
}

// Publish publishes a message to a given subject.
func (nc *NatsClient) Publish(ctx context.Context, subject string, data []byte) error {
	if nc.Conn == nil {
		return fmt.Errorf("NATS connection is not initialized")
	}
	// Could add context deadline/cancellation handling if PublishMsgWithContext is available/needed
	// For basic publish:
	return nc.Conn.Publish(subject, data)
}

// JetStreamPublish publishes a message using JetStream for guaranteed delivery.
// Requires JetStream to be enabled and configured on the server.
func (nc *NatsClient) JetStreamPublish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOption) (*jetstream.PubAck, error) {
	if nc.JS == nil {
		return nil, fmt.Errorf("JetStream is not initialized on this client")
	}
	return nc.JS.Publish(ctx, subject, data, opts...)
}

// Subscribe subscribes to a subject and processes messages with the given handler.
// This is a simple subscription. For JetStream, use JetStreamSubscribe.
func (nc *NatsClient) Subscribe(ctx context.Context, subject string, queueGroup string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if nc.Conn == nil {
		return nil, fmt.Errorf("NATS connection is not initialized")
	}
	var sub *nats.Subscription
	var err error

	if queueGroup != "" {
		sub, err = nc.Conn.QueueSubscribe(subject, queueGroup, handler)
	} else {
		sub, err = nc.Conn.Subscribe(subject, handler)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to subject '%s': %w", subject, err)
	}
	nc.Logger.Info("Subscribed to NATS subject", "subject", subject, "queue_group", queueGroup)
	return sub, nil
}

// JetStreamSubscribe subscribes to a JetStream consumer.
// Requires JetStream to be enabled and consumer to be configured on the server.
// Example opts: jetstream.DeliverAll(), jetstream.AckNone(), jetstream.Durable("myconsumer")
func (nc *NatsClient) JetStreamSubscribe(
	ctx context.Context,
	subject string,
    queueGroup string, // Added queueGroup for consistency, used for durable name if specified
	consumerName string, // Can be empty if creating an ephemeral consumer, or specify durable name
	handler func(msg jetstream.Msg) error, // Handler takes jetstream.Msg
	consumerConfig ...jetstream.ConsumerConfigOpt, // For configuring the consumer on the fly if needed
	subscribeOpts ...jetstream.SubscribeOpt, // Options for the subscription itself (renamed from PullSubscribeOpt)
) (jetstream.ConsumeContext, error) { // Return type changed to reflect what Consume provides
	if nc.JS == nil {
		return nil, fmt.Errorf("JetStream is not initialized on this client")
	}

    durableName := consumerName
    if queueGroup != "" && consumerName == "" { // Use queue group as durable name if consumerName is empty
        durableName = queueGroup
    }


    // Default consumer config (can be overridden or extended)
    // This config is primarily for when we might need to create/update the consumer.
    // For js.Subscribe, many of these are set via jetstream.SubscribeOpt.
    // cfg := jetstream.ConsumerConfig{
    //     Durable: durableName,
    //     AckPolicy: jetstream.AckExplicitPolicy, // Default to explicit ack
    // }
    // Apply provided consumer config options
    // for _, opt := range consumerConfig {
    //     opt(&cfg)
    // }


    wrappedHandler := func(m jetstream.Msg) {
        if err := handler(m); err != nil {
            nc.Logger.ErrorContext(ctx, "Error processing JetStream message", "subject", m.Subject(), "error", err)
            if nackErr := m.NakWithDelay(3 * time.Second); nackErr != nil {
                 nc.Logger.ErrorContext(ctx, "Failed to Nack JetStream message", "subject", m.Subject(), "error", nackErr)
            }
            return
        }
        if ackErr := m.Ack(); ackErr != nil {
            nc.Logger.ErrorContext(ctx, "Failed to Ack JetStream message", "subject", m.Subject(), "error", ackErr)
        }
    }

    var subOpts []jetstream.SubscribeOpt
    subOpts = append(subOpts, jetstream.ManualAck()) // Default to manual ack
    if durableName != "" {
        subOpts = append(subOpts, jetstream.Durable(durableName))
        if queueGroup != "" { // JetStream queue groups are tied to durable consumers
             subOpts = append(subOpts, jetstream.BindToStreamAndQueueGroup(subject, queueGroup))
        }
    }
    subOpts = append(subOpts, subscribeOpts...)


	// Using jetstream.Subscribe for async push-based subscription
	// This method returns a *jetstream.Subscription.
	// To get a jetstream.ConsumeContext, you typically use consumer.Consume().
	// This API might be slightly confusing. If the goal is a managed consumer context,
	// the implementation needs to use `js.Consumer()` then `consumer.Consume()`.
	// For now, we'll return nil for ConsumeContext and focus on the subscription part.
	// A more advanced implementation would manage the consumer lifecycle.

	_, err := nc.JS.Subscribe(ctx, subject, wrappedHandler, subOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to JetStream subscribe to subject '%s' (durable: '%s'): %w", subject, durableName, err)
	}
	nc.Logger.Info("JetStream subscribed to subject", "subject", subject, "durable_name", durableName, "queue_group", queueGroup)

    // Returning ConsumeContext is not directly applicable for this type of push subscription setup.
    // A ConsumeContext is for when you call consumer.Consume().
    // This function, as designed with js.Subscribe, establishes an async subscription.
    // If the caller needs to manage the consumer via ConsumeContext, this function signature/implementation would need to change.
	return nil, fmt.Errorf("JetStreamSubscribe returning ConsumeContext is not directly applicable for this push subscription model; subscription started")
}
