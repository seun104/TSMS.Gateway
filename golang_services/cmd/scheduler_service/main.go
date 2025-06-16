package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fmt"
	"net"
	"golang.org/x/sync/errgroup"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	// Platform packages
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"

	// Scheduler service specific packages
	"github.com/AradIT/aradsms/golang_services/internal/scheduler_service/app"
	grpcadapter "github.com/AradIT/aradsms/golang_services/internal/scheduler_service/adapters/grpc"
	"github.com/AradIT/aradsms/golang_services/internal/scheduler_service/repository/postgres"
	pb "github.com/AradIT/aradsms/golang_services/api/proto/schedulerservice"
)

const (
	serviceName     = "scheduler-service"
	// startupTimeout  = 30 * time.Second // Unused
	shutdownTimeout = 15 * time.Second // Standardized
)

func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("Starting service...")

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"grpc_port", cfg.SchedulerServiceGRPCPort,
		"scheduler_polling_interval", cfg.SchedulerPollingInterval.String(), // Log duration as string
		"scheduler_job_batch_size", cfg.SchedulerJobBatchSize,
	)

	// Initialize Database
	dbPool, err := database.NewDBPool(mainCtx, cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Database connection pool initialized")

	// Initialize NATS Client (Critical for this service)
	if cfg.NATSURL == "" {
		appLogger.Error("NATS URL not configured (APP_NATS_URL). This is critical for scheduler service.")
		os.Exit(1)
	}
	natsClient, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "url", cfg.NATSURL, "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("NATS client connected", "url", cfg.NATSURL)

	// Setup application components
	scheduledJobRepo := postgres.NewPgScheduledJobRepository(dbPool, appLogger)
	pollerCfg := app.PollerConfig{
		PollingInterval: cfg.SchedulerPollingInterval,
		JobBatchSize:    cfg.SchedulerJobBatchSize,
		MaxRetry:        cfg.SchedulerMaxRetry, // Assuming this field exists in config
	}
	jobPoller := app.NewJobPoller(scheduledJobRepo, natsClient, appLogger, pollerCfg)

	// Initialize gRPC server
	schedulerServerAdapter := grpcadapter.NewSchedulerServer(scheduledJobRepo, appLogger)
	grpcServer := grpc.NewServer()
	pb.RegisterSchedulerServiceServer(grpcServer, schedulerServerAdapter)
	reflection.Register(grpcServer)

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start the main scheduling loop (Job Poller)
	g.Go(func() error {
		appLogger.Info("Starting scheduler job poller worker", "polling_interval", pollerCfg.PollingInterval.String())
		// The JobPoller.Run method should internally handle the ticker and respect groupCtx.Done()
		if runErr := jobPoller.Run(groupCtx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			appLogger.Error("Job poller worker failed", "error", runErr)
			return runErr // Propagate error to errgroup
		}
		appLogger.Info("Job poller worker stopped.")
		return nil
	})

	// Start gRPC server goroutine
	g.Go(func() error {
		listenAddress := fmt.Sprintf(":%d", cfg.SchedulerServiceGRPCPort)
		appLogger.Info("gRPC server starting", "address", listenAddress)
		lis, err := net.Listen("tcp", listenAddress)
		if err != nil {
			appLogger.Error("Failed to listen for gRPC", "address", listenAddress, "error", err)
			return fmt.Errorf("failed to listen for gRPC on %s: %w", listenAddress, err)
		}
		// No defer lis.Close() here, as grpcServer.Serve will use it.
		// It will be closed when grpcServer.Serve returns.

		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			appLogger.Error("gRPC server failed to serve", "error", err)
			return err
		}
		appLogger.Info("gRPC server Serve method returned.") // Usually means stopped
		return nil
	})

	// Goroutine for graceful gRPC server shutdown
	g.Go(func() error {
		<-groupCtx.Done() // Wait for shutdown signal
		appLogger.Info("Initiating graceful shutdown of gRPC server...")
		grpcServer.GracefulStop()
		appLogger.Info("gRPC server has been shut down gracefully.")
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
		case <-groupCtx.Done():
			return nil // Context already cancelled
		}
	})

	appLogger.Info("Service is ready and running.")

	// Wait for all goroutines in the group to complete
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, grpc.ErrServerStopped) {
			appLogger.Error("Service group encountered an error", "error", err)
		}
	}
	appLogger.Info("Service shutdown complete.")
}
