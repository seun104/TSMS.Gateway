package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time" // Added for graceful shutdown

	// Adjust the import path according to your go.mod module name
	"github.com/aradsms/golang_services/api/proto/userservice" // Generated proto
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/database" // Assuming this will be implemented
	"github.com/aradsms/golang_services/internal/platform/logger"
	// "github.com/aradsms/golang_services/internal/platform/messagebroker" // For NATS if used by AuthService directly
	"github.com/aradsms/golang_services/internal/user_service/app"
	grpcadapter "github.com/aradsms/golang_services/internal/user_service/adapters/grpc" // gRPC server adapter
	"github.com/aradsms/golang_services/internal/user_service/repository/postgres" // PG repository implementations

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // For gRPC server reflection (optional, useful for debugging)
	"github.com/jackc/pgx/v5/pgxpool" // Added
)

func main() {
	// Load configuration
	// Config path is relative to the binary's execution path.
	// For `go run ./cmd/user_service/main.go` from `golang_services/`, path is "./configs"
	// For binary in `golang_services/bin/user_service` running from `golang_services/`, path is "./configs"
	cfg, err := config.Load("./configs", "config.defaults") // Adjusted path
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("User service starting...", "port (grpc)", cfg.UserServiceGRPCPort, "log_level", cfg.LogLevel)


	// Initialize Database (PostgreSQL) connection
	// Ensure POSTGRES_DSN is set in your config.defaults.yaml or environment
	// Example DSN: "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable"
	if cfg.PostgresDSN == "" {
		appLogger.Error("PostgreSQL DSN is not configured (APP_POSTGRES_DSN)")
		os.Exit(1)
	}
	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL database")

	// Initialize NATS connection (if AuthService needs it directly, otherwise it's for other services)
	// if cfg.NATSUrl == "" {
	// 	appLogger.Error("NATS URL is not configured (APP_NATS_URL)")
	// 	os.Exit(1)
	// }
	// natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "user-service", appLogger)
	// if err != nil {
	//    appLogger.Error("Failed to connect to NATS", "error", err)
	//    os.Exit(1)
	// }
	// defer natsClient.Close()
	// appLogger.Info("Successfully connected to NATS")


	// Initialize Repositories
	userRepo := postgres.NewPgUserRepository(dbPool)
	roleRepo := postgres.NewPgRoleRepository(dbPool)
	permRepo := postgres.NewPgPermissionRepository(dbPool)
	refreshTokenRepo := postgres.NewPgRefreshTokenRepository(dbPool)


	// Initialize AuthService (App Layer)
    // TODO: JWT secrets and expiries should come from config
    authCfg := app.AuthConfig{
        JWTAccessSecret:      cfg.JWTAccessSecret, // Add to AppConfig and config.defaults.yaml
        JWTRefreshSecret:     cfg.JWTRefreshSecret, // Add to AppConfig and config.defaults.yaml
        JWTAccessExpiryHours: cfg.JWTAccessExpiryHours, // Add
        JWTRefreshExpiryHours: cfg.JWTRefreshExpiryHours, // Add
    }
	authService := app.NewAuthService(userRepo, roleRepo, permRepo, refreshTokenRepo, authCfg, appLogger)


	// Setup gRPC Server
	grpcServer := grpc.NewServer(
		// TODO: Add interceptors for logging, metrics, panic recovery, auth (if needed at this layer)
		// grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
		//     grpc_recovery.UnaryServerInterceptor(),
		//     grpc_logging.UnaryServerInterceptor(InterceptorLogger(appLogger)),
		//     // other interceptors
		// )),
	)
	authGRPCServer := grpcadapter.NewAuthGRPCServer(authService, appLogger, authCfg.JWTAccessSecret)
	userservice.RegisterAuthServiceInternalServer(grpcServer, authGRPCServer)

	// Enable server reflection (useful for grpcurl and other tools)
	reflection.Register(grpcServer)

	// Start gRPC server in a goroutine
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.UserServiceGRPCPort)) // Get port from config
	if err != nil {
		appLogger.Error("Failed to listen for gRPC", "port", cfg.UserServiceGRPCPort, "error", err)
		os.Exit(1)
	}
	appLogger.Info(fmt.Sprintf("User service gRPC server listening on port %d", cfg.UserServiceGRPCPort))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("gRPC server failed to serve", "error", err)
			// Consider a mechanism to signal the main goroutine to exit
		}
	}()


	// Graceful shutdown
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)

	receivedSignal := <-quitChan
	appLogger.Info("Shutdown signal received", "signal", receivedSignal.String())

	// Perform graceful shutdown
	appLogger.Info("Attempting graceful shutdown of gRPC server...")
	grpcServer.GracefulStop() // Gracefully stop the gRPC server
	appLogger.Info("gRPC server stopped gracefully.")

	// Close database pool (already deferred, but explicit can be here too if not deferred)
	// Close NATS connection (if natsClient was initialized)

	appLogger.Info("User service shut down successfully.")
}

// --- Add these fields to AppConfig in golang_services/internal/platform/config/config.go ---
// UserServiceGRPCPort  int    `mapstructure:"USER_SERVICE_GRPC_PORT"`
// PostgresDSN          string `mapstructure:"POSTGRES_DSN"`
// NATSUrl              string `mapstructure:"NATS_URL"`
// JWTAccessSecret      string `mapstructure:"JWT_ACCESS_SECRET"`
// JWTRefreshSecret     string `mapstructure:"JWT_REFRESH_SECRET"`
// JWTAccessExpiryHours int    `mapstructure:"JWT_ACCESS_EXPIRY_HOURS"`
// JWTRefreshExpiryHours int   `mapstructure:"JWT_REFRESH_EXPIRY_HOURS"`


// --- And add corresponding defaults/examples to golang_services/configs/config.defaults.yaml ---
// USER_SERVICE_GRPC_PORT: 50051
// POSTGRES_DSN: "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable" # Ensure this matches docker-compose
// NATS_URL: "nats://localhost:4222"
// JWT_ACCESS_SECRET: "your_very_secret_access_key_env_override" # MUST BE OVERRIDDEN BY ENV
// JWT_REFRESH_SECRET: "your_very_secret_refresh_key_env_override" # MUST BE OVERRIDDEN BY ENV
// JWT_ACCESS_EXPIRY_HOURS: 1
// JWT_REFRESH_EXPIRY_HOURS: 720 # 30 days


// --- And update the placeholder database.NewDBPool in golang_services/internal/platform/database/postgres.go ---
// (This part is illustrative; the actual NewDBPool should be implemented properly)
/*
package database

import (
	"context"
	"fmt"
	// "os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDBPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgxpool config: %w", err)
	}

	config.MaxConns = 10 // Example: Set max connections
	config.MinConns = 2  // Example: Set min connections
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30
    config.HealthCheckPeriod = time.Minute // Periodically check health of connections

	pool, err := pgxpool.NewWithConfig(ctx, config) // Use NewWithConfig
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
        pool.Close() // Close the pool if ping fails
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
*/
