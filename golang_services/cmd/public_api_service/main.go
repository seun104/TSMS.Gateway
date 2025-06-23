package main

import (
	"context"
	"encoding/json" // For health check JSON
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database" // For dbPool
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/middleware"
	httptransport "github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http" // Alias for clarity if needed elsewhere
	apphttp "github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http" // Specific alias for http handlers + metrics

	// Import for OutboxRepository implementation
	outboxRepoImpl "github.com/AradIT/aradsms/golang_services/internal/sms_sending_service/repository/postgres"
	phonebookPb "github.com/AradIT/aradsms/golang_services/api/proto/phonebookservice"     // Phonebook gRPC client
	schedulerClientAdapter "github.com/AradIT/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients" // Scheduler gRPC client adapter

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10" // Import validator
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp" // For promhttp.Handler()
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// Blank import for Prometheus metrics registration if using promauto in metrics_middleware
	_ "github.com/AradIT/aradsms/golang_services/internal/public_api_service/transport/http"
)

const serviceName = "public_api_service"

func main() {
	cfg, err := config.Load(serviceName) // Use serviceName for context if config loader uses it
	if err != nil {
		slog.Error("Failed to load configuration", "service", serviceName, "error", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("Public API service starting...", "port", cfg.PublicAPIServicePort)

    // Initialize DB Pool (needed by MessageHandler for OutboxRepository)
    dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
    if err != nil {
        appLogger.Error("Failed to connect to PostgreSQL for public-api", "error", err)
        os.Exit(1)
    }
    defer dbPool.Close()
    appLogger.Info("Public API service connected to PostgreSQL database")


	userSvcClient, err := grpc_clients.NewUserServiceClient(context.Background(), cfg.UserServiceGRPCClientTarget, appLogger)
	if err != nil {
        appLogger.Error("Failed to connect to user service", "error", err); os.Exit(1)
    }
	appLogger.Info("Successfully connected to user gRPC service.")

	// Connect to Phonebook Service
	appLogger.Info("Connecting to Phonebook Service...", "target", cfg.PhonebookServiceGRPCClientTarget)
	phonebookConn, err := grpc.Dial(
		cfg.PhonebookServiceGRPCClientTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure credentials for local dev
		grpc.WithBlock(), // Block until connection is up or times out
	)
	if err != nil {
		appLogger.Error("Failed to connect to Phonebook service", "error", err)
		os.Exit(1)
	}
	defer phonebookConn.Close()
	phonebookClient := phonebookPb.NewPhonebookServiceClient(phonebookConn)
	appLogger.Info("Successfully connected to Phonebook gRPC service.")

	// Connect to Scheduler Service
	// Assuming cfg.SchedulerServiceGRPCClientTarget (e.g., "localhost:50053") exists in public-api-service's config.Config
	schedulerTargetString := cfg.SchedulerServiceGRPCClientTarget
	if schedulerTargetString == "" { // Fallback or error if not configured
		appLogger.Warn("SchedulerService GRPC client target not configured, using default localhost:50053")
		schedulerTargetString = "localhost:50053" // Default target
		// Alternatively, could os.Exit(1) if it's a critical dependency without a sensible default
	}
	schedulerServiceClient, err := schedulerClientAdapter.NewSchedulerServiceClient(schedulerTargetString, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize SchedulerService client", "error", err)
		os.Exit(1)
	}
	defer schedulerServiceClient.Close()
	appLogger.Info("SchedulerService client initialized.", "target", schedulerTargetString)


	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "public-api-service", appLogger, false)
	if err != nil {
        appLogger.Error("Failed to connect to NATS", "error", err)
    }
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS")

	// NATS Test Subscriber (from previous step)
	if natsClient != nil {
		_, err := natsClient.Subscribe(context.Background(), "user.created", "public_api_worker_group", func(msg *nats.Msg) {
            appLogger.Info("NATS message received", "subject", msg.Subject, "data", string(msg.Data))
            // ... (rest of subscriber logic)
        })
		if err != nil {appLogger.Error("Failed to subscribe to NATS 'user.created'", "error", err) }
	}

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)    // Example: add basic request logging
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))
	// Add Prometheus Metrics Middleware
	r.Use(apphttp.PrometheusMetricsMiddleware)


	authMW := middleware.AuthMiddleware(userSvcClient, appLogger)

    // Initialize Repositories needed by handlers in this service
    outboxRepo := outboxRepoImpl.NewPgOutboxRepository(dbPool, appLogger) // Pass dbPool and appLogger

	// Initialize Handlers
	authHandler := apphttp.NewAuthHandler(userSvcClient, appLogger)
    messageHandler := apphttp.NewMessageHandler(natsClient, outboxRepo, dbPool, appLogger)
	// Initialize validator
	validate := validator.New()
    incomingHandler := apphttp.NewIncomingHandler(natsClient, appLogger, validate)
    phonebookHandler := apphttp.NewPhonebookHandler(phonebookClient, appLogger, validate)
	schedulerHandler := apphttp.NewSchedulerHandler(schedulerServiceClient.GetClient(), appLogger, validate)
	exportHandler := apphttp.NewExportHandler(natsClient, appLogger, validate) // Initialize ExportHandler


	// Health check and metrics endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "Public API service is healthy"})
    })
	r.Handle("/metrics", promhttp.Handler()) // Expose Prometheus metrics

	r.Route("/auth", func(authRouter chi.Router) {
		authHandler.RegisterRoutes(authRouter)
	})

    // Message routes (protected)
    r.Group(func(msgRouter chi.Router) {
        msgRouter.Use(authMW) // Apply auth middleware to all message routes
        messageHandler.RegisterRoutes(msgRouter)
    })

    // Incoming callback routes
    r.Route("/incoming", func(incomingRouter chi.Router) {
        // Potentially add IP filtering or other non-JWT auth here if needed
        incomingRouter.Post("/receive/{provider_name}", incomingHandler.HandleDLRCallback)
        incomingRouter.Post("/sms/{provider_name}", incomingHandler.HandleIncomingSMSCallback)
    })

    // API v1 routes (protected by JWT Auth by default)
    r.Route("/api/v1", func(v1Router chi.Router) {
        v1Router.Use(authMW) // Apply auth middleware to all /api/v1 routes

        v1Router.Route("/phonebooks", func(pbRouter chi.Router) {
            phonebookHandler.RegisterRoutes(pbRouter) // Register phonebook CRUD routes
            // Contact routes would be nested under /phonebooks/{phonebookID}/contacts within phonebookHandler.RegisterRoutes
        })

        v1Router.Route("/scheduled_messages", func(sr chi.Router) {
            schedulerHandler.RegisterRoutes(sr)
        })

        v1Router.Route("/exports", func(er chi.Router) {
            er.Post("/outbox-messages", exportHandler.RequestExportOutboxMessages)
        })

        // User profile route (example of another protected route)
        v1Router.Get("/users/me", func(w http.ResponseWriter, r *http.Request) {
            authUser, ok := r.Context().Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
            if !ok {
                appLogger.ErrorContext(r.Context(), "AuthenticatedUser not found in context for /users/me")
                http.Error(w, "Could not retrieve authenticated user", http.StatusInternalServerError)
                return
            }
            profile := apphttp.UserProfileResponse{ // Use apphttp alias
                ID: authUser.ID, Username: authUser.Username, RoleID: authUser.RoleID,
                IsAdmin: authUser.IsAdmin, IsActive: authUser.IsActive, Permissions: authUser.Permissions,
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(profile)
        })
    })


	// Note: The previous /users/me group is now part of /api/v1
	// If any routes are truly public (not /auth, not /incoming, not /api/v1), they'd be here.

	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", cfg.PublicAPIServicePort), Handler: r}
	appLogger.Info("Public API server starting to listen", "address", httpServer.Addr)

	// Start HTTP server in a goroutine
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("HTTP server ListenAndServe failed", "error", err)
			// Consider this a fatal error for the service if the HTTP server can't run.
			// For simplicity here, just logging. In a real scenario, might trigger mainCancel().
		}
	}()

	// Graceful shutdown handling
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)

	<-stopSignal // Wait for termination signal

	appLogger.Info("Shutdown signal received, initiating graceful shutdown of HTTP server...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("HTTP server graceful shutdown failed", "error", err)
	} else {
		appLogger.Info("HTTP server shut down gracefully.")
	}

	appLogger.Info("Public API service shut down.")
}
