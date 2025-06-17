package main

import (
	"context"
	"fmt"
	"errors"
	"log/slog"
	"net"
	"net/http" // For Prometheus metrics server
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AradIT/aradsms/golang_services/api/proto/userservice"
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/user_service/app"
	grpcadapter "github.com/AradIT/aradsms/golang_services/internal/user_service/adapters/grpc"
	"github.com/AradIT/aradsms/golang_services/internal/user_service/repository/postgres"

	"github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	serviceName = "user_service"
	defaultMetricsPort = 9092
)

func main() {
	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("User service starting...")

	// Determine metrics port
	metricsPort := cfg.UserServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("User service metrics port not configured, using default", "port", metricsPort)
	}

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"grpc_port", cfg.UserServiceGRPCPort,
		"metrics_port", metricsPort,
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
	)

	// Initialize Database
	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL database")

	// Initialize NATS connection
	if cfg.NATSURL == "" {
		appLogger.Error("NATS URL is not configured (APP_NATS_URL). This is critical for user service.")
		os.Exit(1)
	}
	natsClient, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "url", cfg.NATSURL, "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS", "url", cfg.NATSURL)

	// Initialize Repositories
	userRepo := postgres.NewPgUserRepository(dbPool, appLogger) // Pass appLogger
	roleRepo := postgres.NewPgRoleRepository(dbPool, appLogger) // Pass appLogger
	permRepo := postgres.NewPgPermissionRepository(dbPool, appLogger) // Pass appLogger
	refreshTokenRepo := postgres.NewPgRefreshTokenRepository(dbPool, appLogger) // Pass appLogger

	authCfg := app.AuthConfig{
		JWTAccessSecret:       cfg.JWTAccessSecret,
		JWTRefreshSecret:      cfg.JWTRefreshSecret,
		JWTAccessExpiryHours:  cfg.JWTAccessExpiryHours,
		JWTRefreshExpiryHours: cfg.JWTRefreshExpiryHours,
	}
	// Pass NATS client to AuthService
	authService := app.NewAuthService(userRepo, roleRepo, permRepo, refreshTokenRepo, natsClient, authCfg, appLogger)

	// Initialize Prometheus gRPC metrics
	grpcMetrics := prometheus.NewServerMetrics(
		prometheus.WithServerHandlingTimeHistogram(
			prometheus.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	// grpcprom.EnableHandlingTimeHistogram() // Deprecated, use NewServerMetrics

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcMetrics.UnaryServerInterceptor(),
			// Add other unary interceptors here
		),
		grpc.ChainStreamInterceptor(
			grpcMetrics.StreamServerInterceptor(),
			// Add other stream interceptors here
		),
	)
	// Register gRPC services
	authGRPCServer := grpcadapter.NewAuthGRPCServer(authService, appLogger, authCfg.JWTAccessSecret)
	userservice.RegisterAuthServiceInternalServer(grpcServer, authGRPCServer)
	reflection.Register(grpcServer)
	grpcMetrics.InitializeMetrics(grpcServer) // Initialize metrics for the server

	// Start gRPC server
	grpcListenAddress := fmt.Sprintf(":%d", cfg.UserServiceGRPCPort)
	lis, err := net.Listen("tcp", grpcListenAddress)
	if err != nil {
		appLogger.Error("Failed to listen for gRPC", "address", grpcListenAddress, "error", err)
		os.Exit(1)
	}
	appLogger.Info("User service gRPC server listening", "address", grpcListenAddress)

	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			appLogger.Error("gRPC server failed to serve", "error", err)
			// Consider this fatal by calling mainCancel() or os.Exit(1) if appropriate
		}
	}()

	// Start Prometheus metrics HTTP server
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", metricsPort),
		Handler: promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}),
	}
	go func() {
		appLogger.Info("Metrics HTTP server listening", "address", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("Metrics HTTP server failed to serve", "error", err)
			// Consider this fatal by calling mainCancel() or os.Exit(1)
		}
	}()

	// Graceful shutdown handling
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)

	<-stopSignal // Wait for termination signal

	appLogger.Info("Shutdown signal received. Attempting graceful shutdown...")

	// Shutdown metrics server
	metricsShutdownCtx, cancelMetricsShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelMetricsShutdown()
	if err := metricsServer.Shutdown(metricsShutdownCtx); err != nil {
		appLogger.Error("Metrics HTTP server graceful shutdown failed", "error", err)
	} else {
		appLogger.Info("Metrics HTTP server shut down gracefully.")
	}

	// Shutdown gRPC server
	appLogger.Info("Attempting graceful shutdown of gRPC server...")
	grpcServer.GracefulStop() // This will block until completed
	appLogger.Info("gRPC server stopped gracefully.")

	appLogger.Info("User service shut down successfully.")
}
