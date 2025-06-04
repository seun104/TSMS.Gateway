package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/nats-io/nats.go"
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
	startupTimeout = 30 * time.Second
	shutdownTimeout = 10 * time.Second
	pollingInterval = 30 * time.Second // How often to poll for DLRs
)

func main() {
	// Main context for startup and long-running operations until shutdown signal
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Initialize Logger
	log := logger.New(serviceName, "dev") // Or load level from config
	log.Info("Starting service...")

	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	log.Info("Configuration loaded successfully")
	// Example: log.Info("NATS URL", "url", cfg.NATS.URL)

	// Initialize Database (PostgreSQL)
	dbPool, err := database.NewPostgresPool(ctx, cfg.Postgres.DSN, log)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("Database connection pool initialized")

	// Initialize NATS Connection
	nc, err := messagebroker.NewNATSClient(cfg.NATS.URL, log, serviceName)
	if err != nil {
		log.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	log.Info("NATS connection initialized")

	// Setup application components (e.g., repositories, services, consumers)
	// Example:
	// deliveryApp := app.NewDeliveryApplication(deliveryRepo, nc, log, cfg) // This would likely take DLRPoller or its components

	// Initialize Repositories
	outboxRepo := postgres.NewPgOutboxRepository(dbPool, log)

	// Initialize Application Services
	dlrPoller := app.NewDLRPoller(log)
	dlrProcessor := app.NewDLRProcessor(outboxRepo, log)

	// Start background workers, consumers, servers
	// Use errgroup with the mainCtx for goroutines that should run until shutdown
	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start the DLR Poller ticker
	g.Go(func() error {
		log.Info("Starting DLR poller worker...")
		ticker := time.NewTicker(pollingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Use a new context for each poll operation with a timeout
				pollCtx, pollCancel := context.WithTimeout(groupCtx, pollingInterval- (5 * time.Second) ) // Ensure poll timeout is less than interval
				reports, pollErr := dlrPoller.PollProvider(pollCtx)
				if pollErr != nil {
					log.ErrorContext(pollCtx, "Error polling for DLRs", "error", pollErr)
					// Continue to next tick even if polling fails
				} else {
					if len(reports) > 0 {
						log.InfoContext(pollCtx, "Successfully polled DLRs", "count", len(reports))
						// Process the DLRs
						if processErr := dlrProcessor.ProcessDLRs(pollCtx, reports); processErr != nil {
							log.ErrorContext(pollCtx, "Error processing DLRs", "error", processErr)
						}
					} else {
						log.InfoContext(pollCtx, "No new DLRs from poll.")
					}
				}
				pollCancel() // Release resources associated with pollCtx
			case <-groupCtx.Done(): // Triggered by mainCancel() or if another goroutine in group fails
				log.Info("DLR poller worker stopping due to group context done.", "error", groupCtx.Err())
				return groupCtx.Err()
			}
		}
	})

	// Example: Start a NATS consumer (if this service also consumes messages)
	// g.Go(func() error {
	//	 log.Info("Starting NATS consumer...")
	//	 return deliveryApp.StartDLRConsumer(groupCtx) // Pass groupCtx
	// })

	log.Info("Service components initialized and workers started. Service is ready.")

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received or a goroutine in the group errors
	var groupErr error
	select {
	case sig := <-sigCh:
		log.Info("Received termination signal", "signal", sig)
	case groupErr = <-watchGroup(g): // watchGroup waits for the errgroup to complete
		log.Error("A critical component failed, initiating shutdown", "error", groupErr)
	}

	// Initiate graceful shutdown
	log.Info("Attempting graceful shutdown...")
	mainCancel() // Signal all goroutines in the errgroup to stop

	// Create a new context for shutdown with a timeout
	shutdownCtx, shutdownCancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancelTimeout()

	// Wait for all goroutines in the group to finish.
	// If groupErr was from watchGroup, g.Wait() will return it again.
	// If shutdown was initiated by a signal, g.Wait() will return errors from goroutines if they failed during shutdown.
	if err := g.Wait(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Error("Error during graceful shutdown of components", "error", err)
	} else if groupErr != nil && groupErr != context.Canceled && groupErr != context.DeadlineExceeded {
		// Log the original error that caused the shutdown if it wasn't a signal
		log.Error("Shutdown initiated due to component error", "error", groupErr)
	}


	// Add other specific shutdown logic here if needed (e.g., closing resources not managed by the errgroup)
	// dbPool.Close() and nc.Close() are deferred already.

	log.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
// It returns the error that caused the group to exit.
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		// g.Wait() blocks until all goroutines in the group have completed or the context is cancelled.
		// It returns the first non-nil error returned by a goroutine, or nil if all completed successfully.
		errCh <- g.Wait()
	}()
	return errCh
}

// Placeholder for actual config structure if needed
// type ServiceConfig struct {
// 	Postgres database.PostgresConfig
// 	NATS     messagebroker.NATSConfig
// 	// Add other service-specific configs
// }
//
// func loadConfig(log *slog.Logger) (*ServiceConfig, error) {
// 	// This is a simplified example.
// 	// In a real app, you'd use Viper or similar to load from file/env.
//  // cfg, err := config.Load("delivery_retrieval_service")
// 	// if err != nil {
// 	// 	return nil, err
// 	// }
// 	// return &cfg.SpecificServiceConfig, nil // Assuming config.Load returns a general struct
// 	return nil, fmt.Errorf("config loading for specific service needs to be adapted")
// }

// Note: The config loading part (`cfg, err := config.Load(serviceName)`)
// might need adjustment if `config.Load` is generic and doesn't directly
// return a structure with `cfg.Postgres.DSN` and `cfg.NATS.URL`.
// The current placeholder `config.Load` is assumed to provide these.
// If `ServiceConfig` struct above were used, `LoadConfig` would populate it,
// and `main` would call `loadConfig(log)` instead of `config.Load(serviceName)`.
// For this task, we'll assume the existing config loading works as is for DSN and NATS URL.
