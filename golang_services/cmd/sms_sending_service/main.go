package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	// "time"

	"github.com/aradsms/golang_services/api/proto/billingservice"
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/database"
	"github.com/aradsms/golang_services/internal/platform/logger"
	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/aradsms/golang_services/internal/sms_sending_service/app"
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository/postgres"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // For dev
    // "github.com/jackc/pgx/v5/pgxpool" // Not directly needed here
)

const (
	natsSMSJobSubject   = "sms.jobs.send"
	natsSMSJobQueueGroup = "sms_sending_workers"
)

func main() {
	cfg, err := config.Load("./configs", "config.defaults")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("SMS Sending Service starting...", "log_level", cfg.LogLevel)

	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL database")

	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "sms-sending-service", appLogger, false) // false for JetStream initially
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS")

	// Initialize gRPC client for Billing Service
    if cfg.BillingServiceGRPCClientTarget == "" { // Add this to config
        appLogger.Error("Billing service gRPC client target URL is not configured (APP_BILLING_SERVICE_GRPC_CLIENT_TARGET)")
        os.Exit(1)
    }
	billingConn, err := grpc.DialContext(context.Background(), cfg.BillingServiceGRPCClientTarget, // Get from config
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		appLogger.Error("Failed to connect to billing service", "error", err, "target", cfg.BillingServiceGRPCClientTarget)
		os.Exit(1)
	}
	defer billingConn.Close()
	billingServiceClient := billingservice.NewBillingInternalServiceClient(billingConn)
	appLogger.Info("Successfully connected to Billing gRPC service.")


	outboxRepo := postgres.NewPgOutboxRepository()
	// Using MockSMSProvider for now
	mockProvider := provider.NewMockSMSProvider(appLogger, false, 0) // No fail, no delay

	smsAppService := app.NewSMSSendingAppService(outboxRepo, mockProvider, billingServiceClient, natsClient, dbPool, appLogger)

	// Start consuming NATS jobs
    // The context passed to StartConsumingJobs should ideally be one that can be cancelled on shutdown.
    // For simplicity, context.Background() is used, but a cancellable context is better for goroutine management.
    appCtx, cancelAppCtx := context.WithCancel(context.Background())
    defer cancelAppCtx()


	if err := smsAppService.StartConsumingJobs(appCtx, natsSMSJobSubject, natsSMSJobQueueGroup); err != nil {
		appLogger.Error("Failed to start NATS job consumer", "error", err)
		os.Exit(1) // If consumer can't start, service is not functional
	}
	appLogger.Info("NATS consumer started", "subject", natsSMSJobSubject, "queue_group", natsSMSJobQueueGroup)


	// Keep the service running & handle graceful shutdown
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-quitChan
	appLogger.Info("Shutdown signal received", "signal", receivedSignal.String())

    cancelAppCtx() // Signal the app context to cancel, helping to stop ongoing operations like NATS subscription loop

	appLogger.Info("Attempting graceful shutdown of SMS Sending Service...")
	smsAppService.StopConsumingJobs() // Unsubscribe from NATS
	// Other cleanup (dbPool, natsClient, billingConn are deferred)
	appLogger.Info("SMS Sending Service shut down successfully.")
}

// --- Add BillingServiceGRPCClientTarget to AppConfig in golang_services/internal/platform/config/config.go ---
// BillingServiceGRPCClientTarget string `mapstructure:"BILLING_SERVICE_GRPC_CLIENT_TARGET"`
// --- And add to golang_services/configs/config.defaults.yaml ---
// BILLING_SERVICE_GRPC_CLIENT_TARGET: "billing-service:50052" // Internal K8s/Docker Compose DNS name
