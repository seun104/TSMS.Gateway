package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"log/slog" // Added for appLogger
	"net/http" // For Prometheus metrics server
	"errors"   // For http.ErrServerClosed

	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/app" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/repository/postgres" // Corrected path
	// "github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/adapters/some_adapter" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	_ "github.com/AradIT/aradsms/golang_services/internal/delivery_retrieval_service/app" // Blank import for promauto metrics
)

const (
	serviceName         = "delivery_retrieval_service"
	defaultMetricsPort  = 9096 // Default port for Prometheus metrics
	shutdownTimeout     = 10 * time.Second
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

	// Determine metrics port
	metricsPort := cfg.DeliveryRetrievalServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("Delivery Retrieval service metrics port not configured, using default", "port", metricsPort)
	}

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"log_level", cfg.LogLevel,
		"metrics_port", metricsPort,
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

	// Start Metrics HTTP Server Goroutine
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", metricsPort),
		Handler: metricsMux,
	}

	g.Go(func() error {
		appLogger.Info("Metrics HTTP server starting", "address", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("Metrics HTTP server ListenAndServe error", "error", err)
			return err
		}
		appLogger.Info("Metrics HTTP server shut down gracefully.")
		return nil
	})

	// Goroutine for handling termination signals
	g.Go(func() error {
		stopSignal := make(chan os.Signal, 1)
		signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-stopSignal:
			appLogger.Info("Received termination signal", "signal", sig.String())
			mainCancel() // Initiate shutdown for all components
			return nil
		case <-groupCtx.Done(): // If any other goroutine in the group errors out or mainCancel is called
			return nil // Context already cancelled
		}
	})

	// Goroutine for graceful shutdown of the metrics server
	g.Go(func() error {
		<-groupCtx.Done() // Wait for cancellation
		appLogger.Info("Initiating graceful shutdown of Metrics HTTP server...")

		shutdownCtx, cancelMetricsShutdown := context.WithTimeout(context.Background(), 5*time.Second) // Short timeout
		defer cancelMetricsShutdown()

		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("Metrics HTTP server graceful shutdown failed", "error", err)
		} else {
			appLogger.Info("Metrics HTTP server shut down successfully.")
		}
		return nil // Error from shutdown is logged, not propagated to errgroup to avoid premature exit if other components are fine.
	})


	appLogger.Info("Service components initialized and workers started. Service is ready.")

	// Wait for all goroutines in the group to complete
	if err := g.Wait(); err != nil {
		// Log errors that are not expected during a graceful shutdown
		if !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("Service group encountered an error", "error", err)
		}
	}
	appLogger.Info("Service shutdown complete.")
}
