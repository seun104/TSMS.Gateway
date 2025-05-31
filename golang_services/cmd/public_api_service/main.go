package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Adjust import paths
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/logger"
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
	// Placeholder for actual handlers/routes
	// httphandlers "github.com/aradsms/golang_services/internal/public_api_service/transport/http"

	"github.com/go-chi/chi/v5" // Using Chi router as an example
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg, err := config.Load("./configs", "config.defaults") // Relative to where binary runs
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("Public API service starting...", "port", cfg.PublicAPIServicePort, "log_level", cfg.LogLevel)

	// Initialize User Service gRPC client
	if cfg.UserServiceGRPCClientTarget == "" { // Add this to config
		appLogger.Error("User service gRPC client target URL is not configured")
		os.Exit(1)
	}
	userSvcClient, err := grpc_clients.NewUserServiceClient(context.Background(), cfg.UserServiceGRPCClientTarget, appLogger)
	if err != nil {
		appLogger.Error("Failed to connect to user service", "error", err)
		os.Exit(1)
	}
	appLogger.Info("Successfully connected to user gRPC service.")


	// Setup Router (using Chi as an example)
	r := chi.NewRouter()

	// Base Middlewares
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	// r.Use(chimiddleware.Logger) // Chi's logger, or use a custom slog one
    r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second)) // Set a reasonable timeout

	// Auth Middleware (applied to protected routes)
	authMW := middleware.AuthMiddleware(userSvcClient, appLogger)

	// Public routes (e.g., for login, registration - to be defined in transport/http)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Public API service is healthy"))
	})
    // authHandler := httphandlers.NewAuthHandler(userSvcClient, appLogger, cfg.SomeAuthConfigForAPI) // Example
    // r.Mount("/auth", authHandler.Routes())


	// Protected routes group
	r.Group(func(protected chi.Router) {
		protected.Use(authMW)
		// Example protected route
		protected.Get("/users/me", func(w http.ResponseWriter, r *http.Request) {
			authUser, ok := r.Context().Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
			if !ok {
				http.Error(w, "Could not retrieve authenticated user", http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "Hello, %s! Your ID is %s. Is Admin: %t", authUser.Username, authUser.ID, authUser.IsAdmin)
		})

        // Example of using permission middleware
        // permCheckMW := middleware.BasicPermissionCheckMiddleware("some:permission", appLogger)
        // protected.With(permCheckMW).Post("/some/resource", someResourceHandler.Create)

	})


	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.PublicAPIServicePort), // Get port from config
		Handler: r,
	}
	appLogger.Info(fmt.Sprintf("Public API server listening on port %d", cfg.PublicAPIServicePort))

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("HTTP server failed to serve", "error", err)
		}
	}()

	// Graceful shutdown
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	<-quitChan
	appLogger.Info("Shutdown signal received, shutting down HTTP server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30-second timeout for shutdown
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		appLogger.Error("HTTP server shutdown failed", "error", err)
	} else {
		appLogger.Info("HTTP server shut down gracefully.")
	}
}

// --- Add these fields to AppConfig in golang_services/internal/platform/config/config.go ---
// PublicAPIServicePort      int    `mapstructure:"PUBLIC_API_SERVICE_PORT"`
// UserServiceGRPCClientTarget string `mapstructure:"USER_SERVICE_GRPC_CLIENT_TARGET"`


// --- And add corresponding defaults/examples to golang_services/configs/config.defaults.yaml ---
// PUBLIC_API_SERVICE_PORT: 8080
// USER_SERVICE_GRPC_CLIENT_TARGET: "localhost:50051" # Or "dns:///user-service:50051" in K8s
