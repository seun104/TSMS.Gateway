package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"log/slog" // Added for appLogger

	"github.com/nats-io/nats.go" // pgxpool import removed as it's not directly used in main after db init
	"golang.org/x/sync/errgroup"

	"github.com/your-repo/project/internal/delivery_retrieval_service/app" // Adjusted path
	"github.com/your-repo/project/internal/delivery_retrieval_service/repository/postgres" // Import repository implementation
	// "github.com/your-repo/project/internal/delivery_retrieval_service/adapters/some_adapter"
	"github.com/your-repo/project/internal/platform/config"
	"github.com/your-repo/project/internal/platform/database"
	"github.com/your-repo/project/internal/platform/logger"
	"github.com/your-repo/project/internal/platform/messagebroker"
)

const (
	serviceName = "delivery_retrieval_service"
	// startupTimeout = 30 * time.Second // Already part of platform/config or unused directly
	shutdownTimeout = 10 * time.Second
	// pollingInterval = 30 * time.Second // How often to poll for DLRs - part of app logic now
)

func main() {
	// Main context for startup and long-running operations until shutdown signal
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Load Configuration First
	cfg, err := config.Load(serviceName)
	if err != nil {
		// Use a temporary basic logger if config load fails before appLogger is initialized
		slog.Error("Failed to load configuration", "error", err, "service", serviceName)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("Starting service...")

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"log_level", cfg.LogLevel,
	)

	// Initialize Database (PostgreSQL)
	// Assuming NewDBPool is the correct function as per billing_service
	dbPool, err := database.NewDBPool(mainCtx, cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Database connection pool initialized")

	// Initialize NATS Connection
	nc, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	appLogger.Info("NATS connection initialized")

	// Setup application components
	outboxRepo := postgres.NewPgOutboxRepository(dbPool, appLogger)
	dlrProcessor := app.NewDLRProcessor(outboxRepo, nc, appLogger)

	dlrEventsChan := make(chan app.DLREvent, 100)
	dlrConsumer := app.NewDLRConsumer(nc, appLogger, dlrEventsChan)

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start the NATS DLR Consumer
	g.Go(func() error {
		appLogger.Info("Starting NATS DLR consumer worker", "subject", "dlr.raw.*", "queue_group", "dlr_processor_group")
		// Subject "dlr.raw.*" captures DLRs from all providers
		// Queue group "dlr_processor_group" ensures load balancing
		return dlrConsumer.StartConsuming(groupCtx, "dlr.raw.*", "dlr_processor_group")
	})

	// Start a worker goroutine to process DLR events from dlrEventsChan
	g.Go(func() error {
		appLogger.Info("Starting DLR event processor worker...")
		for {
			select {
			case event := <-dlrEventsChan:
				if err := dlrProcessor.ProcessDLREvent(groupCtx, event); err != nil {
					appLogger.Error("Failed to process DLR event",
						"error", err,
						"provider", event.ProviderName,
						"provider_message_id", event.RequestData.ProviderMessageID,
						"original_message_id", event.RequestData.MessageID,
					)
					// Error already logged by DLRProcessor, this adds context if needed or could be removed if redundant
				}
			case <-groupCtx.Done():
				appLogger.Info("DLR event processor worker shutting down.", "error", groupCtx.Err())
				return groupCtx.Err()
			}
		}
	})

	// Old mock DLRPoller logic is assumed to be removed or commented out as per original file.
	// If it were active, its logging would also need to use appLogger.

	appLogger.Info("Service components initialized and workers started. Service is ready.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var groupErr error
	select {
	case sig := <-sigCh:
		appLogger.Info("Received termination signal", "signal", sig.String())
	case groupErr = <-watchGroup(g):
		appLogger.Error("A critical component failed, initiating shutdown", "error", groupErr)
	}

	appLogger.Info("Attempting graceful shutdown...")
	mainCancel()

	// shutdownCtx, shutdownCancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout) // This context is not used below
	// defer shutdownCancelTimeout()

	if err := g.Wait(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		appLogger.Error("Error during graceful shutdown of components", "error", err)
	} else if groupErr != nil && groupErr != context.Canceled && groupErr != context.DeadlineExceeded {
		appLogger.Error("Shutdown initiated due to component error", "error", groupErr)
	}

	appLogger.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
// It returns the error that caused the group to exit.
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()
	return errCh
}
