package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	// "time"

	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice"
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/app"
	appdomain "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/domain" // Alias to avoid conflict if needed
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/provider"
	"github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/repository/postgres"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // For dev
    // "github.com/jackc/pgx/v5/pgxpool" // Not directly needed here
)

const (
	serviceName          = "sms-sending-service"
	natsSMSJobSubject    = appdomain.NatsSMSJobSendSubject // Using domain const
	natsSMSJobQueueGroup = appdomain.NatsSMSJobSendQueueGroup // Using domain const
)

func main() {
	// Load Configuration
	cfg, err := config.Load(serviceName)
	if err != nil {
		// Use default slog if appLogger not yet initialized
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	// Initialize Logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger = appLogger.With("service", serviceName)
	appLogger.Info("SMS Sending Service starting...")

	// Log Key Configuration Details
	appLogger.Info("Configuration loaded",
		"log_level", cfg.LogLevel,
		"nats_url", cfg.NATSURL,
		"postgres_dsn_present", cfg.PostgresDSN != "",
		"billing_service_target", cfg.BillingServiceGRPCClientTarget,
		"default_provider", cfg.SMSSendingServiceDefaultProvider,
	)

	// Application context for managing component lifecycles
	appCtx, cancelAppCtx := context.WithCancel(context.Background())
	defer cancelAppCtx()

	// Initialize Database
	// Using context.Background() for initial resource setup like DB pool
	// appCtx is for longer-lived processes like consumers/servers.
	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL database")

	// Initialize NATS Client
	if cfg.NATSURL == "" {
		appLogger.Error("NATS URL not configured (APP_NATS_URL). This is critical for SMS Sending service.")
		os.Exit(1)
	}
	natsClient, err := messagebroker.NewNATSClient(cfg.NATSURL, appLogger, serviceName)
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "url", cfg.NATSURL, "error", err)
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS", "url", cfg.NATSURL)

	// Initialize Billing Service gRPC Client
	if cfg.BillingServiceGRPCClientTarget == "" {
		appLogger.Error("Billing service gRPC client target URL is not configured (APP_BILLING_SERVICE_GRPC_CLIENT_TARGET)")
		os.Exit(1)
	}
	// Use appCtx for gRPC client connection context
	billingConn, err := grpc.DialContext(appCtx, cfg.BillingServiceGRPCClientTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		appLogger.Error("Failed to connect to billing service", "error", err, "target", cfg.BillingServiceGRPCClientTarget)
		os.Exit(1)
	}
	defer billingConn.Close()
	billingServiceClient := billingservice.NewBillingInternalServiceClient(billingConn)
	appLogger.Info("Successfully connected to Billing gRPC service.")

	// Initialize Repositories
	outboxRepo := postgres.NewPgOutboxRepository(dbPool, appLogger)
	appLogger.Info("OutboxRepository initialized.")

	// Initialize Providers
	providers := make(map[string]appdomain.SMSSenderProvider) // Use appdomain for SMSSenderProvider
	mockProvider := provider.NewMockSMSProvider(appLogger, false, 0) // provider.NewMockSMSProvider is correct
	providers[mockProvider.GetName()] = mockProvider
	appLogger.Info("MockSMSProvider initialized.")

	if cfg.MagfaProviderAPIURL != "" && cfg.MagfaProviderAPIKey != "" && cfg.MagfaProviderSenderID != "" && cfg.MagfaProviderAPIURL != "https_magfa_api_url_here" {
		magfaProvider := provider.NewMagfaSMSProvider(appLogger, cfg.MagfaProviderAPIURL, cfg.MagfaProviderAPIKey, cfg.MagfaProviderSenderID, nil) // provider.NewMagfa is correct
		providers[magfaProvider.GetName()] = magfaProvider
		appLogger.Info("MagfaSMSProvider initialized.")
	} else {
		appLogger.Warn("MagfaSMSProvider not initialized due to missing or default placeholder configuration.")
	}

	effectiveDefaultProvider := cfg.SMSSendingServiceDefaultProvider
	if effectiveDefaultProvider == "" {
		effectiveDefaultProvider = mockProvider.GetName()
		appLogger.Warn("Default SMS provider not configured (APP_SMS_SENDING_SERVICE_DEFAULT_PROVIDER), falling back to mock.", "default_provider", effectiveDefaultProvider)
	}

	// Initialize RouteRepository
	routeRepo := postgres.NewPgRouteRepository(dbPool, appLogger.With("component", "route_repository"))
	appLogger.Info("RouteRepository initialized.")

	// Initialize Router
	router := app.NewRouter(routeRepo, providers, appLogger.With("component", "router")) // app.NewRouter is correct
	appLogger.Info("Router initialized.")

	// Initialize BlacklistRepository
	blacklistRepo := postgres.NewPgBlacklistRepository(dbPool, appLogger.With("component", "blacklist_repository"))
	appLogger.Info("BlacklistRepository initialized.")

	// Initialize FilterWordRepository
	filterWordRepo := postgres.NewPgFilterWordRepository(dbPool, appLogger.With("component", "filter_word_repository"))
	appLogger.Info("FilterWordRepository initialized.")

	// Initialize SMSSendingAppService
	smsAppService := app.NewSMSSendingAppService( // app.NewSMSSendingAppService is correct
		outboxRepo,
		providers,
		effectiveDefaultProvider,
		billingServiceClient,
		natsClient,
		dbPool,
		appLogger,
		router,
		blacklistRepo,
		filterWordRepo,
	)
	appLogger.Info("SMSSendingAppService initialized.")

	// Start consuming NATS jobs
	// The existing structure starts this synchronously in main flow after setup,
	// and handles shutdown via signal and calling StopConsumingJobs.
	// This is kept as per subtask instruction not to make it part of errgroup unless trivial.
	if err := smsAppService.StartConsumingJobs(appCtx, natsSMSJobSubject, natsSMSJobQueueGroup); err != nil {
		appLogger.Error("Failed to start NATS job consumer", "error", err)
		os.Exit(1) // If consumer cannot start, service is unhealthy
	}
	appLogger.Info("NATS consumer started", "subject", natsSMSJobSubject, "queue_group", natsSMSJobQueueGroup)

	// Wait for termination signal
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	receivedSignal := <-quitChan

	appLogger.Info("Shutdown signal received", "signal", receivedSignal.String())
	appLogger.Info("Attempting graceful shutdown of SMS Sending Service...")

	cancelAppCtx() // Signal appCtx to be done, for components listening to it (like gRPC client if it used appCtx for streams)

	smsAppService.StopConsumingJobs() // Ensure this is idempotent and handles being called after context cancel.

	appLogger.Info("SMS Sending Service shut down successfully.")
}
