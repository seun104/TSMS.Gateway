package main

import (
	"context"
	// "fmt" // Example, if config loading needs it
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	// "github.com/jackc/pgx/v4/pgxpool" // Example, if directly used
	// "github.com/nats-io/nats.go" // Example, if directly used
	"golang.org/x/sync/errgroup"

	// Corrected paths for platform packages
	"github.com/your-repo/project/internal/platform/config"
	"github.com/your-repo/project/internal/platform/database"
	"github.com/your-repo/project/internal/platform/logger"
	"github.com/your-repo/project/internal/platform/messagebroker"

	"github.com/your-repo/project/internal/inbound_processor_service/app" // Import app package
	"github.com/your-repo/project/internal/inbound_processor_service/domain"
	"github.com/your-repo/project/internal/inbound_processor_service/repository/postgres" // Import repository
)

const (
	serviceName     = "inbound_processor_service"
	startupTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	// Main context for startup and long-running operations until shutdown signal
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Initialize Logger
	log := logger.New(serviceName, "dev") // Or load level from config
	log.Info("Starting service...")

	// Load Configuration
	// Assuming config.Load can find config.defaults.yaml or similar from a standard path
	// or that SERVICE_NAME is used to find inbound_processor_service.yaml
	cfg, err := config.Load(serviceName)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	log.Info("Configuration loaded successfully")
	// Example: log.Info("NATS URL", "url", cfg.NATS.URL)
	// Example: log.Info("Postgres DSN", "dsn", cfg.Postgres.DSN)


	// Initialize Database (PostgreSQL)
	dbPool, err := database.NewPostgresPool(mainCtx, cfg.Postgres.DSN, log) // Use mainCtx for initial setup
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("Database connection pool initialized")

	// Initialize NATS Connection
	nc, err := messagebroker.NewNATSClient(cfg.NATS.URL, log, serviceName) // Use serviceName for NATS client name
	if err != nil {
		log.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	log.Info("NATS connection initialized")

	// Setup application components (e.g., repositories, application services, NATS consumers)
	// Example:
	// Initialize Repositories
	inboxRepo := postgres.NewPgInboxRepository(dbPool, log)

	// Initialize Application Services / Components
	// Create a channel for passing messages from consumer to processor logic. Buffer size can be tuned.
	inboundEventsChan := make(chan app.InboundSMSEvent, 100)
	smsConsumer := app.NewSMSConsumer(nc, log, inboundEventsChan)
	smsProcessor := app.NewSMSProcessor(inboxRepo, log)
	// inboundApp := app.NewInboundApplication(smsProcessor, ...) // Higher-level app service if needed

	// Start background workers, NATS consumers, etc.
	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start NATS consumer for raw incoming SMS
	// The subject "sms.incoming.raw.*" allows capturing messages from any provider,
	// assuming the provider name is the last part of the subject.
	g.Go(func() error {
		log.Info("Starting NATS consumer for incoming SMS...")
		// This is a blocking call and will return when ctx is cancelled or an error occurs.
		return smsConsumer.StartConsuming(groupCtx, "sms.incoming.raw.*", "inbound_processor_group")
	})

	// Start a worker goroutine to process messages from inboundEventsChan
	g.Go(func() error {
		log.Info("Starting inbound SMS event processor worker...")
		for {
			select {
			case event := <-inboundEventsChan:
				// Process the received event using SMSProcessor
				if err := smsProcessor.ProcessMessage(groupCtx, event); err != nil {
					log.ErrorContext(groupCtx, "Failed to process inbound SMS event",
						"error", err,
						"provider", event.ProviderName,
						"provider_message_id", event.Data.MessageID,
					)
					// Depending on the error, might implement retry or dead-lettering.
				}
			case <-groupCtx.Done():
				log.InfoContext(groupCtx, "Inbound SMS event processor worker shutting down.", "error", groupCtx.Err())
				return groupCtx.Err()
			}
		}
	})

	log.Info("Service components initialized and workers (if any) started. Service is ready.")

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var groupErr error
	select {
	case sig := <-sigCh:
		log.Info("Received termination signal", "signal", sig)
	case groupErr = <-watchGroup(g):
		log.Error("A critical component failed, initiating shutdown", "error", groupErr)
	}

	// Initiate graceful shutdown
	log.Info("Attempting graceful shutdown...")
	mainCancel() // Signal all goroutines in the errgroup to stop

	shutdownCtx, shutdownCancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancelTimeout()

	if err := g.Wait(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Error("Error during graceful shutdown of components", "error", err)
	} else if groupErr != nil && groupErr != context.Canceled && groupErr != context.DeadlineExceeded {
		log.Error("Shutdown initiated due to component error", "error", groupErr)
	}

	log.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()
	return errCh
}
