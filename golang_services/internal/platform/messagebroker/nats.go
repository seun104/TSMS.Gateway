package messagebroker

import (
	// "log/slog"
	// "time"

	// "github.com/nats-io/nats.go"
	// "github.com/nats-io/nats.go/jetstream"
)

// NatsClient wraps NATS connection and JetStream context
// type NatsClient struct {
// 	Conn *nats.Conn
// 	JS   jetstream.JetStream
//  Logger *slog.Logger
// }

// NewNatsClient connects to NATS and optionally sets up JetStream.
// natsURL example: "nats://localhost:4222" or "tls://user:pass@localhost:4222"
// func NewNatsClient(natsURL string, appName string, logger *slog.Logger) (*NatsClient, error) {
// 	nc, err := nats.Connect(natsURL,
// 		nats.Name(appName),
//      nats.Timeout(5*time.Second),
//      nats.PingInterval(20*time.Second),
// 		nats.MaxPingsOutstanding(3),
// 		nats.ReconnectWait(2*time.Second),
// 		nats.MaxReconnects(-1), // Infinite reconnects
//      nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
// 			logger.Warn("NATS disconnected", "error", err)
// 		}),
// 		nats.ReconnectHandler(func(nc *nats.Conn) {
// 			logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
// 		}),
// 		nats.ClosedHandler(func(nc *nats.Conn) {
// 			logger.Error("NATS connection closed", "error", nc.LastError())
// 		}),
//  )
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
// 	}

// 	js, err := jetstream.New(nc)
// 	if err != nil {
//      nc.Close()
// 		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
// 	}

// 	return &NatsClient{Conn: nc, JS: js, Logger: logger}, nil
// }

// Close closes the NATS connection.
// func (nc *NatsClient) Close() {
// 	if nc.Conn != nil && !nc.Conn.IsClosed() {
// 		nc.Conn.Drain() // Drain ensures all published messages are sent
// 		nc.Conn.Close()
// 	}
// }

// Placeholder comment:
// TODO: Implement NATS connection and JetStream setup
// Ensure NATS URL is loaded from config/env
// Implement helper functions for publish/subscribe if needed
