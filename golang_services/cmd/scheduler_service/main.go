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
	"net/http" // For Prometheus metrics server
	"golang.org/x/sync/errgroup"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	// Prometheus
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"

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

	// Blank import for promauto metrics registration in app package
	_ "github.com/AradIT/aradsms/golang_services/internal/scheduler_service/app"
)

const (
	serviceName         = "scheduler-service"
	defaultMetricsPort  = 9095 // Default port for Prometheus metrics
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
	// Determine metrics port
	metricsPort := cfg.SchedulerServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("Scheduler service metrics port not configured, using default", "port", metricsPort)
	}

	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"grpc_port", cfg.SchedulerServiceGRPCPort,
		"metrics_port", metricsPort,
		"scheduler_polling_interval", cfg.SchedulerPollingInterval.String(),
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

	jobPoller := app.NewJobPoller(scheduledJobRepo, natsClient, appLogger, pollerCfg)

	// Initialize Prometheus gRPC metrics
	grpcMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	if err := prometheus.DefaultRegisterer.Register(grpcMetrics); err != nil {
		appLogger.Warn("Failed to register gRPC Prometheus metrics", "error", err)
	}

	// Initialize gRPC server
	schedulerServerAdapter := grpcadapter.NewSchedulerServer(scheduledJobRepo, appLogger)
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcMetrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(grpcMetrics.StreamServerInterceptor()),
	)
	pb.RegisterSchedulerServiceServer(grpcServer, schedulerServerAdapter)
	reflection.Register(grpcServer)
	grpcMetrics.InitializeMetrics(grpcServer) // Initialize metrics for the server

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

	// Goroutine for graceful shutdown of servers (gRPC and Metrics)
	g.Go(func() error {
		<-groupCtx.Done() // Wait for shutdown signal

		appLogger.Info("Initiating graceful shutdown of Metrics HTTP server...")
		metricsShutdownCtx, cancelMetricsShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelMetricsShutdown()
		if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
			appLogger.Error("Metrics HTTP server graceful shutdown failed", "error", err)
			// Potentially collect this error if needed
		} else {
			appLogger.Info("Metrics HTTP server shut down successfully.")
		}

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
		if !errors.Is(err, context.Canceled) && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("Service group encountered an error", "error", err)
		}
	}
	appLogger.Info("Service shutdown complete.")
}
