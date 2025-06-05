package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	// Platform packages
	"github.com/your-repo/project/internal/platform/config"
	"github.com/your-repo/project/internal/platform/database"
	"github.com/your-repo/project/internal/platform/logger"
	"github.com/your-repo/project/internal/platform/messagebroker"

	// Scheduler service specific packages (placeholders)
	// "github.com/your-repo/project/internal/scheduler_service/app"
	// "github.com/your-repo/project/internal/scheduler_service/repository/postgres"
)

const (
	serviceName     = "scheduler_service"
	startupTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	log := logger.New(serviceName, "dev") // Or load level from config
	log.Info("Starting service...")

	cfg, err := config.Load(serviceName)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	log.Info("Configuration loaded successfully")
	// Example: log.Info("NATS URL", "url", cfg.NATS.URL)
	// Example: log.Info("Postgres DSN", "dsn", cfg.Postgres.DSN)

	dbPool, err := database.NewPostgresPool(mainCtx, cfg.Postgres.DSN, log)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("Database connection pool initialized")

	natsClient, err := messagebroker.NewNATSClient(cfg.NATS.URL, log, serviceName)
	if err != nil {
		log.Error("Failed to connect to NATS", "error", err) // NATS is critical for publishing jobs
		os.Exit(1)
	}
	defer natsClient.Close()
	log.Info("NATS connection initialized")

	// Setup application components
	// scheduledJobRepo := postgres.NewPgScheduledJobRepository(dbPool, log)
	// schedulerApp := app.NewSchedulerApplication(scheduledJobRepo, natsClient, log, cfg) // App to manage scheduling logic

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start the main scheduling loop/ticker
	// g.Go(func() error {
	//	 log.Info("Starting scheduler loop...")
	//	 return schedulerApp.RunScheduler(groupCtx)
	// })

	log.Info("Service components initialized and workers (if any) started. Service is ready.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var groupErr error
	select {
	case sig := <-sigCh:
		log.Info("Received termination signal", "signal", sig)
	case groupErr = <-watchGroup(g):
		log.Error("A critical component failed, initiating shutdown", "error", groupErr)
	}

	log.Info("Attempting graceful shutdown...")
	mainCancel()

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
