package main

import (
	"context"
	"log/slog" // Keep slog for appLogger
	"net/http" // For Prometheus metrics server
	"os"
	"os/signal"
	"syscall"
	"time"
	"errors" // For http.ErrServerClosed

	"golang.org/x/sync/errgroup"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// Corrected paths for platform packages
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"

	"github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/app" // Import app package
	_ "github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/domain" // Domain types used via app typically
	"github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/repository/postgres" // Import repository

	// Blank import for promauto metrics registration in app package
	_ "github.com/AradIT/aradsms/golang_services/internal/inbound_processor_service/app" // Corrected path
)

const (
	serviceName         = "inbound_processor_service"
	defaultMetricsPort  = 9098 // Default port for Prometheus metrics
	// startupTimeout  = 30 * time.Second // Unused in main directly
	shutdownTimeout = 10 * time.Second
)

func main() {
	// Main context for startup and long-running operations until shutdown signal
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		// Use default slog if appLogger not yet initialized
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("Starting service...")

	// Determine metrics port
	metricsPort := cfg.InboundProcessorServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("Inbound Processor service metrics port not configured, using default", "port", metricsPort)
	}

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"metrics_port", metricsPort,
	)

	// Initialize Database (PostgreSQL)
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
	inboxRepo := postgres.NewPgInboxRepository(dbPool, appLogger)
	privateNumRepo := postgres.NewPgPrivateNumberRepository(dbPool, appLogger)

	inboundEventsChan := make(chan app.InboundSMSEvent, 100)
	smsConsumer := app.NewSMSConsumer(nc, appLogger, inboundEventsChan)
	smsProcessor := app.NewSMSProcessor(
		inboxRepo,
		privateNumRepo,
		nc, // Pass the NATS client
		appLogger.With("component", "sms_processor"),
	)

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start NATS consumer for raw incoming SMS
	g.Go(func() error {
		appLogger.Info("Starting NATS consumer for incoming SMS", "subject", "sms.incoming.raw.*", "queue_group", "inbound_processor_group")
		return smsConsumer.StartConsuming(groupCtx, "sms.incoming.raw.*", "inbound_processor_group")
	})

	// Start a worker goroutine to process messages from inboundEventsChan
	g.Go(func() error {
		appLogger.Info("Starting inbound SMS event processor worker...")
		for {
			select {
			case event := <-inboundEventsChan:
				if err := smsProcessor.ProcessMessage(groupCtx, event); err != nil {
					// Using slog attributes for structured logging
					appLogger.Error("Failed to process inbound SMS event",
						slog.Any("error", err), // Use slog.Any for errors or err.Error() for string
						slog.String("provider", event.ProviderName),
						slog.String("provider_message_id", event.Data.MessageID),
					)
				}
			case <-groupCtx.Done():
				errMsg := "nil"
				if groupCtx.Err() != nil {
					errMsg = groupCtx.Err().Error()
				}
				appLogger.Info("Inbound SMS event processor worker shutting down.", slog.String("error", errMsg))
				return groupCtx.Err()
			}
		}
	})

	appLogger.Info("Service components initialized and workers (if any) started. Service is ready.")

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

	// Removed unused shutdownCtx and shutdownCancelTimeout
	// shutdownCtx, shutdownCancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	// defer shutdownCancelTimeout()

	if err := g.Wait(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		appLogger.Error("Error during graceful shutdown of components", "error", err)
	} else if groupErr != nil && groupErr != context.Canceled && groupErr != context.DeadlineExceeded {
		appLogger.Error("Shutdown initiated due to component error", "error", groupErr)
	}

	appLogger.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()
	return errCh
}
