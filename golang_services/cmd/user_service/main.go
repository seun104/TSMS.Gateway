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

	"github.com/aradsms/golang_services/api/proto/userservice"
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/database"
	"github.com/aradsms/golang_services/internal/platform/logger"
	"github.com/aradsms/golang_services/internal/platform/messagebroker" // Added NATS
	"github.com/aradsms/golang_services/internal/user_service/app"
	grpcadapter "github.com/aradsms/golang_services/internal/user_service/adapters/grpc"
	"github.com/aradsms/golang_services/internal/user_service/repository/postgres"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	// "github.com/jackc/pgx/v5/pgxpool" // No longer directly needed here
)

func main() {
	cfg, err := config.Load("./configs", "config.defaults")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("User service starting...", "port (grpc)", cfg.UserServiceGRPCPort, "log_level", cfg.LogLevel)

	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL database")

	// Initialize NATS connection
	if cfg.NATSUrl == "" {
		appLogger.Error("NATS URL is not configured (APP_NATS_URL)")
		os.Exit(1)
	}
	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "user-service", appLogger, false) // false for useJetStream initially
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer natsClient.Close() // Ensure NATS client is closed on shutdown
	appLogger.Info("Successfully connected to NATS")

	userRepo := postgres.NewPgUserRepository(dbPool)
	roleRepo := postgres.NewPgRoleRepository(dbPool)
	permRepo := postgres.NewPgPermissionRepository(dbPool)
	refreshTokenRepo := postgres.NewPgRefreshTokenRepository(dbPool)

	authCfg := app.AuthConfig{
		JWTAccessSecret:       cfg.JWTAccessSecret,
		JWTRefreshSecret:      cfg.JWTRefreshSecret,
		JWTAccessExpiryHours:  cfg.JWTAccessExpiryHours,
		JWTRefreshExpiryHours: cfg.JWTRefreshExpiryHours,
	}
	// Pass NATS client to AuthService
	authService := app.NewAuthService(userRepo, roleRepo, permRepo, refreshTokenRepo, natsClient, authCfg, appLogger)

	grpcServer := grpc.NewServer()
	authGRPCServer := grpcadapter.NewAuthGRPCServer(authService, appLogger, authCfg.JWTAccessSecret)
	userservice.RegisterAuthServiceInternalServer(grpcServer, authGRPCServer)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.UserServiceGRPCPort))
	if err != nil {
		appLogger.Error("Failed to listen for gRPC", "port", cfg.UserServiceGRPCPort, "error", err)
		os.Exit(1)
	}
	appLogger.Info(fmt.Sprintf("User service gRPC server listening on port %d", cfg.UserServiceGRPCPort))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("gRPC server failed to serve", "error", err)
		}
	}()

	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-quitChan
	appLogger.Info("Shutdown signal received", "signal", receivedSignal.String())

	appLogger.Info("Attempting graceful shutdown of gRPC server...")
	grpcServer.GracefulStop()
	appLogger.Info("gRPC server stopped gracefully.")
	appLogger.Info("User service shut down successfully.")
}
