package main

import (
	"context"
	"encoding/json" // For decoding NATS message
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/logger"
	"github.com/aradsms/golang_services/internal/platform/messagebroker" // NATS client
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
	httptransport "github.com/aradsms/golang_services/internal/public_api_service/transport/http" // Alias for http transport

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go" // For nats.MsgHandler
)

func main() {
	cfg, err := config.Load("./configs", "config.defaults")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("Public API service starting...", "port", cfg.PublicAPIServicePort, "log_level", cfg.LogLevel)

	userSvcClient, err := grpc_clients.NewUserServiceClient(context.Background(), cfg.UserServiceGRPCClientTarget, appLogger)
	if err != nil {
		appLogger.Error("Failed to connect to user service", "error", err)
		os.Exit(1)
	}
	appLogger.Info("Successfully connected to user gRPC service.")

	// Initialize NATS Client for this service (e.g., for subscribing to events)
	natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "public-api-service", appLogger, false) // false for useJetStream
	if err != nil {
		appLogger.Error("Failed to connect to NATS", "error", err)
		// Potentially non-critical for API service to start if only subscribing for optional features
		// For this test, let's make it critical.
		os.Exit(1)
	}
	defer natsClient.Close()
	appLogger.Info("Successfully connected to NATS")

	// NATS Test Subscriber for user.created
	if natsClient != nil {
		_, err := natsClient.Subscribe(context.Background(), "user.created", "public_api_worker_group", func(msg *nats.Msg) {
			appLogger.Info("NATS message received", "subject", msg.Subject, "data", string(msg.Data))
			var userEventPayload map[string]string
			if err := json.Unmarshal(msg.Data, &userEventPayload); err != nil {
				appLogger.Error("Failed to unmarshal NATS user.created event payload", "error", err, "data", string(msg.Data))
				return
			}
			appLogger.Info("User created event processed", "user_id", userEventPayload["user_id"], "username", userEventPayload["username"])
			// In a real app, you might update a local cache, send a websocket notification, etc.
		})
		if err != nil {
			appLogger.Error("Failed to subscribe to NATS subject 'user.created'", "error", err)
			// Potentially non-critical, log and continue
		} else {
			appLogger.Info("Subscribed to NATS subject 'user.created'")
		}
	}


	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))
    // Custom Slog middleware for request logging (example)
    // r.Use(SlogRequestLogger(appLogger))


	authMW := middleware.AuthMiddleware(userSvcClient, appLogger)
	authHandler := httptransport.NewAuthHandler(userSvcClient, appLogger) // Use alias

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "Public API service is healthy"})
	})

	r.Route("/auth", func(authRouter chi.Router) {
		authHandler.RegisterRoutes(authRouter)
	})

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
				ID:          authUser.ID,
				Username:    authUser.Username,
				// Email:    authUser.Email, // Add Email to AuthenticatedUser if it's populated by ValidateToken
				RoleID:      authUser.RoleID,
				IsAdmin:     authUser.IsAdmin,
				IsActive:    authUser.IsActive,
				Permissions: authUser.Permissions,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(profile)
		})
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.PublicAPIServicePort),
		Handler: r,
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		appLogger.Error("HTTP server shutdown failed", "error", err)
	} else {
		appLogger.Info("HTTP server shut down gracefully.")
	}
	appLogger.Info("Public API service shut down.")
}

// Optional: SlogRequestLogger middleware example
// func SlogRequestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
// 	return func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			start := time.Now()
// 			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
// 			defer func() {
// 				logger.Info("HTTP Request",
// 					"method", r.Method,
// 					"path", r.URL.Path,
// 					"remote_addr", r.RemoteAddr,
// 					"request_id", chimiddleware.GetReqID(r.Context()),
// 					"status", ww.Status(),
// 					"bytes_written", ww.BytesWritten(),
// 					"duration_ms", float64(time.Since(start).Nanoseconds())/1e6,
// 				)
// 			}()
// 			next.ServeHTTP(ww, r)
// 		})
// 	}
// }
