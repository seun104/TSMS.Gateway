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
	appdomain "github.com/aradsms/golang_services/internal/sms_sending_service/domain" // Alias to avoid conflict if needed
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
	cfg, err := config.Load("./configs", "config.defaults") // Consider making serviceName an argument to Load
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

	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "sms-sending-service", appLogger, false)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS")

	if cfg.BillingServiceGRPCClientTarget == "" {
		appLogger.Error("Billing service gRPC client target URL is not configured (APP_BILLING_SERVICE_GRPC_CLIENT_TARGET)")
		os.Exit(1)
	}
	billingConn, err := grpc.DialContext(context.Background(), cfg.BillingServiceGRPCClientTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		appLogger.Error("Failed to connect to billing service", "error", err, "target", cfg.BillingServiceGRPCClientTarget)
		os.Exit(1)
	}
	defer billingConn.Close()
	billingServiceClient := billingservice.NewBillingInternalServiceClient(billingConn)
	appLogger.Info("Successfully connected to Billing gRPC service.")

	outboxRepo := postgres.NewPgOutboxRepository()
	appLogger.Info("OutboxRepository initialized.")

	providers := make(map[string]provider.SMSSenderProvider)
	mockProvider := provider.NewMockSMSProvider(appLogger, false, 0)
	providers[mockProvider.GetName()] = mockProvider
	appLogger.Info("MockSMSProvider initialized.")

	if cfg.MagfaProviderAPIURL != "" && cfg.MagfaProviderAPIKey != "" && cfg.MagfaProviderSenderID != "" && cfg.MagfaProviderAPIURL != "https_magfa_api_url_here" {
		magfaProvider := provider.NewMagfaSMSProvider(appLogger, cfg.MagfaProviderAPIURL, cfg.MagfaProviderAPIKey, cfg.MagfaProviderSenderID, nil)
		providers[magfaProvider.GetName()] = magfaProvider
		appLogger.Info("MagfaSMSProvider initialized.")
	} else {
		appLogger.Warn("MagfaSMSProvider not initialized due to missing or default placeholder configuration.")
	}

	if cfg.SMSSendingServiceDefaultProvider == "" {
		cfg.SMSSendingServiceDefaultProvider = mockProvider.GetName()
		appLogger.Warn("Default SMS provider not configured, falling back to mock.", "default_provider", cfg.SMSSendingServiceDefaultProvider)
	}

	// Initialize RouteRepository
	routeRepo := postgres.NewPgRouteRepository(dbPool, appLogger.With("component", "route_repository"))
	appLogger.Info("RouteRepository initialized.")

	// Initialize Router
	router := app.NewRouter(routeRepo, providers, appLogger.With("component", "router"))
	appLogger.Info("Router initialized.")

	// Initialize BlacklistRepository
	blacklistRepo := postgres.NewPgBlacklistRepository(dbPool, appLogger.With("component", "blacklist_repository"))
	appLogger.Info("BlacklistRepository initialized.")

	// Initialize SMSSendingAppService
	smsAppService := app.NewSMSSendingAppService(
		outboxRepo,
		providers,
		cfg.SMSSendingServiceDefaultProvider,
		billingServiceClient,
		natsClient,
		dbPool,
		appLogger,
		router,
		blacklistRepo, // Pass the blacklist repository instance
	)
	appLogger.Info("SMSSendingAppService initialized.")

	appCtx, cancelAppCtx := context.WithCancel(context.Background())
	defer cancelAppCtx()

	if err := smsAppService.StartConsumingJobs(appCtx, natsSMSJobSubject, natsSMSJobQueueGroup); err != nil {
		appLogger.Error("Failed to start NATS job consumer", "error", err)
		os.Exit(1)
	}
	appLogger.Info("NATS consumer started", "subject", natsSMSJobSubject, "queue_group", natsSMSJobQueueGroup)

	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-quitChan
	appLogger.Info("Shutdown signal received", "signal", receivedSignal.String())

	cancelAppCtx()

	appLogger.Info("Attempting graceful shutdown of SMS Sending Service...")
	smsAppService.StopConsumingJobs()
	appLogger.Info("SMS Sending Service shut down successfully.")
}
