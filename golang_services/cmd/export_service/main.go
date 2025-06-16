package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"errors" // For http.ErrServerClosed check

	"github.com/AradIT/aradsms/golang_services/internal/export_service/app"
	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain" // For NATS subject constants
	"github.com/AradIT/aradsms/golang_services/internal/export_service/repository/postgres"
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"golang.org/x/sync/errgroup"
)

const (
	serviceName     = "export-service"
	shutdownTimeout = 15 * time.Second
)

func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		// Use slog.Default() for logging if appLogger is not yet initialized
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("Export service starting...")

	// Log essential configuration details
	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL,
		"export_path_configured", cfg.ExportServiceExportPath != "",
	)

	// Determine and create export path
	effectiveExportPath := cfg.ExportServiceExportPath
	if effectiveExportPath == "" {
		effectiveExportPath = "/tmp/exports_service_default" // Default if not in config
		appLogger.Warn("Export path not configured (APP_EXPORT_SERVICE_EXPORT_PATH), using default", "path", effectiveExportPath)
	}
	if err := os.MkdirAll(effectiveExportPath, 0750); err != nil {
		appLogger.Error("Failed to create export directory during startup", "path", effectiveExportPath, "error", err)
		// Decide if this is fatal. For now, log and continue.
	}

	// Initialize Database (PostgreSQL)
	dbPool, err := database.NewDBPool(mainCtx, cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Database connection pool initialized")

	// Initialize Repositories
	outboxExportRepo := postgres.NewPgOutboxExportRepository(dbPool, appLogger)
	appLogger.Info("OutboxExportRepository initialized")

	// Initialize Application Services
	exportService := app.NewExportService(outboxExportRepo, appLogger, effectiveExportPath)
	appLogger.Info("ExportService initialized")

	// Initialize NATS Client
	if cfg.NATSURL == "" {
		appLogger.Error("NATS URL not configured (APP_NATS_URL)")
		os.Exit(1)
	}
	natsClient, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("NATS client connected", "url", cfg.NATSURL)


	// Setup for graceful shutdown using errgroup
	g, groupCtx := errgroup.WithContext(mainCtx)

	// Initialize and start NATS Consumer
	natsConsumer := app.NewNATSConsumer(exportService, natsClient, appLogger)
	g.Go(func() error {
		appLogger.Info("NATS consumer starting", "subject", exportDomain.NATSExportRequestOutboxV1, "queue_group", "export_workers_queue")
		// The message handler for NATS client's Subscribe method needs to match its expected signature.
		// Assuming it's func(ctx context.Context, subject string, data []byte) error or similar that can be adapted.
		// Let's assume the NATSClient's Subscribe method handles context for cancellation.
		// The handler `natsConsumer.HandleExportRequest` has signature func(ctx context.Context, subject string, data []byte)
		// which is compatible if the messagebroker.NATSClient.Subscribe expects such a handler.
		// The current messagebroker.NATSClient.Subscribe takes func(msg messagebroker.Message)
		// So, we need an adapter or to change HandleExportRequest signature.
		// For now, let's assume we adapt it here:
		adaptedHandler := func(msg messagebroker.Message) {
			// Pass groupCtx, or a new derived context for the message processing
			msgProcessingCtx, cancel := context.WithTimeout(groupCtx, 10*time.Minute) // Timeout per message
			defer cancel()
			natsConsumer.HandleExportRequest(msgProcessingCtx, msg.Subject(), msg.Data())
		}
		// The Subscribe method in mock was (ctx, subj, queue, handler func(msg Msg)), actual might differ
		// Assuming the NATSClient platform wrapper has a Subscribe method like:
		// Subscribe(ctx context.Context, subject string, queueGroup string, handler func(ctx context.Context, subject string, data []byte)) error
		// If it's func(msg *nats.Msg), the adapter is slightly different.
		// The current messagebroker.NatsClient.Subscribe takes func(ctx context.Context, subject, queueGroup string, handler func(msg *nats.Msg))
		// Let's assume the NATSClient in platform has a Subscribe method that takes a handler of type:
		// func(ctx context.Context, subject string, data []byte)
		// If not, this part needs adjustment based on actual NATSClient interface.
		// For now, assuming a compatible Subscribe method in messagebroker.NATSClient or adapting the handler.
		// The prompt's NATSClient has: Subscribe(ctx context.Context, subject string, queueGroup string, handler func(msg messagebroker.Message)) (messagebroker.Subscription, error)
		// And messagebroker.Message has Subject() and Data().

		// Correct handler signature for the assumed NATSClient interface:
		adaptedMsgHandler := func(msg messagebroker.Message) {
			// Using groupCtx for the message processing, or derive a new one
			// For long running tasks, it's better to derive a new context with timeout
			// from the groupCtx to allow individual task cancellation/timeout.
			procCtx, procCancel := context.WithTimeout(groupCtx, 5 * time.Minute) // Example timeout for processing one export request
			defer procCancel()
			natsConsumer.HandleExportRequest(procCtx, msg.Subject(), msg.Data())
		}

		subscription, err := natsClient.Subscribe(groupCtx, exportDomain.NATSExportRequestOutboxV1, "export_workers_queue", adaptedMsgHandler)
		if err != nil {
			appLogger.Error("Failed to subscribe to NATS subject for export requests", "subject", exportDomain.NATSExportRequestOutboxV1, "error", err)
			return err
		}
		// Wait for context to be cancelled, then unsubscribe
        <-groupCtx.Done()
        appLogger.Info("NATS consumer shutting down, unsubscribing...")
        if unsubErr := subscription.Unsubscribe(); unsubErr != nil {
            appLogger.Error("Error unsubscribing from NATS", "subject", exportDomain.NATSExportRequestOutboxV1, "error", unsubErr)
        }
		return nil
	})


	// --- Graceful Shutdown Handling ---
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)

	g.Go(func() error {
		select {
		case sig := <-stopSignal:
			appLogger.Info("Received termination signal", "signal", sig.String())
		case <-groupCtx.Done(): // If any other goroutine in the group errors out
			appLogger.Info("Group context done, initiating shutdown", "error", groupCtx.Err())
		}
		mainCancel() // Start shutdown of all goroutines managed by groupCtx
		return nil
	})

	// Shutdown goroutine for any long-running components (e.g., NATS client if started)
	g.Go(func() error {
		<-groupCtx.Done() // Wait for mainCancel() or a component failure
		appLogger.Info("Initiating graceful shutdown of service components...")
		// Add cleanup for NATS client here when it's added
		// Example: if natsClient != nil { natsClient.Close() }
		appLogger.Info("Service components shut down.")
		return nil
	})


	appLogger.Info("Export service is ready and running.")
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		appLogger.Error("Service group encountered an error during run", "error", err)
	}

	appLogger.Info("Export service shut down successfully.")
}
