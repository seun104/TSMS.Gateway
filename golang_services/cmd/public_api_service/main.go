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

	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/database" // For dbPool
	"github.com/aradsms/golang_services/internal/platform/logger"
	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
	httptransport "github.com/aradsms/golang_services/internal/public_api_service/transport/http" // Alias for clarity if needed elsewhere
	incomingHttp "github.com/aradsms/golang_services/internal/public_api_service/transport/http" // Specific alias for incoming handler

	// Import for OutboxRepository implementation
	outboxRepoImpl "github.com/aradsms/golang_services/internal/sms_sending_service/repository/postgres"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10" // Import validator
	"github.com/nats-io/nats.go"
)

func main() {
	cfg, err := config.Load("./configs", "config.defaults")
	if err != nil {
            slog.Error("Failed to load configuration", "error", err); os.Exit(1)
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

	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "public-api-service", appLogger, false)
	if err != nil {
        appLogger.Error("Failed to connect to NATS", "error", err); os.Exit(1)
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
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	authMW := middleware.AuthMiddleware(userSvcClient, appLogger)

    // Initialize Repositories needed by handlers in this service
    outboxRepo := outboxRepoImpl.NewPgOutboxRepository() // Instantiating the repo from sms_sending_service's package

	// Initialize Handlers
	authHandler := httptransport.NewAuthHandler(userSvcClient, appLogger)
    messageHandler := httptransport.NewMessageHandler(natsClient, outboxRepo, dbPool, appLogger)
	// Initialize validator
	validate := validator.New()
    incomingHandler := incomingHttp.NewIncomingHandler(natsClient, appLogger, validate)


	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "Public API service is healthy"})
    })

	r.Route("/auth", func(authRouter chi.Router) {
		authHandler.RegisterRoutes(authRouter)
	})

    // Message routes (protected)
    r.Group(func(msgRouter chi.Router) {
        msgRouter.Use(authMW) // Apply auth middleware to all message routes
        messageHandler.RegisterRoutes(msgRouter)
    })

    // Incoming callback routes (typically not authenticated by user JWT, but by IP, secret, or other mechanism)
    // For now, placing it at the root. Consider if it needs a specific path prefix e.g. /callbacks
    r.Post("/incoming/receive/{provider_name}", incomingHandler.HandleDLRCallback)
    r.Post("/incoming/sms/{provider_name}", incomingHandler.HandleIncomingSMSCallback)


	r.Group(func(protected chi.Router) {
		protected.Use(authMW)
		protected.Get("/users/me", func(w http.ResponseWriter, r *http.Request) {
            authUser, ok := r.Context().Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
            if !ok {
                appLogger.ErrorContext(r.Context(), "AuthenticatedUser not found in context for /users/me")
                http.Error(w, "Could not retrieve authenticated user", http.StatusInternalServerError)
                return
            }
            profile := httptransport.UserProfileResponse{
                ID: authUser.ID, Username: authUser.Username, RoleID: authUser.RoleID,
                IsAdmin: authUser.IsAdmin, IsActive: authUser.IsActive, Permissions: authUser.Permissions,
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(profile)
        })
	})

	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", cfg.PublicAPIServicePort), Handler: r}
	appLogger.Info(fmt.Sprintf("Public API server listening on port %d", cfg.PublicAPIServicePort))
    go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("HTTP server failed to serve", "error", err)
		}
	}()
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	<-quitChan
	appLogger.Info("Shutdown signal received, shutting down HTTP server...")
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(ctxShutdown); err != nil {
		appLogger.Error("HTTP server shutdown failed", "error", err)
	} else {
		appLogger.Info("HTTP server shut down gracefully.")
	}
	appLogger.Info("Public API service shut down.")
}
