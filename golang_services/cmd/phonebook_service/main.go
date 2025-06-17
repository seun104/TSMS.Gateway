package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"errors"
	"net/http" // For Prometheus metrics server
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // For gRPC server reflection

	// Prometheus
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"

	// Platform packages
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"

	// Phonebook service specific packages
	pb "github.com/AradIT/aradsms/golang_services/api/proto/phonebookservice" // Generated gRPC code
	grpcAdapter "github.com/AradIT/aradsms/golang_services/internal/phonebook_service/adapters/grpc"    // Your gRPC server implementation
	phonebookApp "github.com/AradIT/aradsms/golang_services/internal/phonebook_service/app"            // Application layer
	"github.com/AradIT/aradsms/golang_services/internal/phonebook_service/repository/postgres" // Repository implementations
)

const (
	serviceName         = "phonebook_service"
	defaultMetricsPort  = 9094 // Default port for Prometheus metrics
	// startupTimeout  = 30 * time.Second // Unused
	shutdownTimeout = 15 * time.Second // Standardized shutdown timeout
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
	metricsPort := cfg.PhonebookServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("Phonebook service metrics port not configured, using default", "port", metricsPort)
	}

	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL, // Will be empty if not set
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"grpc_port", cfg.PhonebookServiceGRPCPort,
		"metrics_port", metricsPort,
	)

	// Initialize Database
	dbPool, err := database.NewDBPool(mainCtx, cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Database connection pool initialized")

	// Initialize NATS Client (Optional)
	if cfg.NATSURL != "" {
		natsClient, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
		if err != nil {
			appLogger.Error("Failed to connect to NATS", "url", cfg.NATSURL, "error", err)
			// Depending on service requirements, this might be a fatal error.
			// For now, assume it's not critical for phonebook if it fails to connect.
		} else {
			defer natsClient.Close()
			appLogger.Info("NATS client connected", "url", cfg.NATSURL)
		}
	} else {
		appLogger.Info("NATS URL not configured, NATS client will not be initialized.")
	}

	// Setup application components
	phonebookRepo := postgres.NewPgPhonebookRepository(dbPool, appLogger)
	contactRepo := postgres.NewPgContactRepository(dbPool, appLogger)
	application := phonebookApp.NewApplication(phonebookRepo, contactRepo, appLogger)
	grpcServerInstance := grpcAdapter.NewGRPCServer(application, appLogger)

	// Initialize Prometheus gRPC metrics
	grpcMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	if err := prometheus.DefaultRegisterer.Register(grpcMetrics); err != nil {
		appLogger.Warn("Failed to register gRPC Prometheus metrics", "error", err)
	}

	// Initialize gRPC server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcMetrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(grpcMetrics.StreamServerInterceptor()),
	)
	pb.RegisterPhonebookServiceServer(grpcServer, grpcServerInstance)
	reflection.Register(grpcServer) // Enable server reflection
	grpcMetrics.InitializeMetrics(grpcServer) // Initialize metrics for the server

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start gRPC server goroutine
	g.Go(func() error {
		listenAddress := fmt.Sprintf(":%d", cfg.PhonebookServiceGRPCPort)
		appLogger.Info("gRPC server starting", "address", listenAddress)
		lis, err := net.Listen("tcp", listenAddress)
		if err != nil {
			appLogger.Error("Failed to listen for gRPC", "address", listenAddress, "error", err)
			return fmt.Errorf("failed to listen for gRPC on %s: %w", listenAddress, err)
		}
		defer lis.Close()

		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			appLogger.Error("gRPC server failed to serve", "error", err)
			return err
		}
		appLogger.Info("gRPC server stopped gracefully.")
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
			return nil // Context already cancelled, just exit
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

	// Goroutine for graceful shutdown of servers (gRPC and Metrics)
	g.Go(func() error {
		<-groupCtx.Done() // Wait for shutdown signal (from mainCancel or other error)

		appLogger.Info("Initiating graceful shutdown of Metrics HTTP server...")
		metricsShutdownCtx, cancelMetricsShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelMetricsShutdown()
		if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
			appLogger.Error("Metrics HTTP server graceful shutdown failed", "error", err)
			// Potentially collect this error if needed, e.g., errors.Join
		} else {
			appLogger.Info("Metrics HTTP server shut down successfully.")
		}

		appLogger.Info("Initiating graceful shutdown of gRPC server...")
		grpcServer.GracefulStop()
		appLogger.Info("gRPC server has been shut down gracefully.")
		return nil
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
