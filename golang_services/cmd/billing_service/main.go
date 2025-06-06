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
	"errors"
	"net/http"


	"github.com/AradIT/aradsms/golang_services/api/proto/billingservice"
	grpcadapter "github.com/AradIT/aradsms/golang_services/internal/billing_service/adapters/grpc"
	httpadapter "github.com/AradIT/aradsms/golang_services/internal/billing_service/adapters/http"
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/app"
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/repository/postgres"
	"github.com/AradIT/aradsms/golang_services/internal/platform/config"
	"github.com/AradIT/aradsms/golang_services/internal/platform/database"
	"github.com/AradIT/aradsms/golang_services/internal/platform/logger"

	// Placeholder for user service client adapter within billing service
	// This adapter would implement the user_service/repository.UserRepository interface
	// For now, we'll assume it's available.
	userserviceclient "github.com/AradIT/aradsms/golang_services/internal/billing_service/adapters/grpc_clients/userservice" // Example path
	userRepoInterface "github.com/AradIT/aradsms/golang_services/internal/user_service/repository" // Interface path

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"golang.org/x/sync/errgroup"
	gRPC "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	serviceName     = "billing-service"
	shutdownTimeout = 15 * time.Second // Shared timeout for graceful shutdown
)


func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	cfg, err := config.Load(serviceName) // Use serviceName for config loading
	if err != nil {
		slog.Error("Failed to load configuration", "error", err); os.Exit(1) // Use default slog if appLogger not yet init
	}
	appLogger := logger.New(cfg.LogLevel) // Initialize early
	appLogger = appLogger.With("service", serviceName) // Add service context to logger
	appLogger.Info("Billing service starting...", "grpc_port", cfg.BillingServiceGRPCPort, "http_port", cfg.BillingServiceHTTPPort, "log_level", cfg.LogLevel)

	dbPool, err := database.NewDBPool(mainCtx, cfg.PostgresDSN, appLogger)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL", "error", err); os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL")

	// Initialize User Service Client (Adapter)
	// This adapter implements userRepository.UserRepository interface by calling User Service via gRPC
	// The actual implementation of this adapter is outside this subtask's scope, assuming it exists.
	if cfg.UserServiceGRPCClientTarget == "" {
		appLogger.Error("User service gRPC client target not configured")
		os.Exit(1)
	}
	userRepoAdapter, err := userserviceclient.NewUserServiceAdapter(cfg.UserServiceGRPCClientTarget, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize User Service client adapter", "error", err); os.Exit(1)
	}
	appLogger.Info("User Service client adapter initialized", "target", cfg.UserServiceGRPCClientTarget)


	transactionRepo := postgres.NewPgTransactionRepository(dbPool, appLogger)
	paymentIntentRepo := postgres.NewPgPaymentIntentRepository(dbPool, appLogger)

	// Initialize Mock Payment Gateway Adapter for now
	// TODO: Replace with actual payment gateway adapter when implemented
	mockPaymentGateway := app.NewMockPaymentGatewayAdapter(appLogger, false, false, false, true) // success events by default
	appLogger.Info("MockPaymentGatewayAdapter initialized")


	billingApp := app.NewBillingService(
		transactionRepo,
		userRepoAdapter, // Pass the adapter that implements the required interface
		paymentIntentRepo,
		mockPaymentGateway,
		dbPool,
		appLogger,
	)
	appLogger.Info("BillingAppService initialized")

	g, groupCtx := errgroup.WithContext(mainCtx)

	// --- Start gRPC Server ---
	grpcServer := gRPC.NewServer()
	billingGRPCServer := grpcadapter.NewBillingGRPCServer(billingApp, appLogger)
	billingservice.RegisterBillingServiceInternalServer(grpcServer, billingGRPCServer)
	reflection.Register(grpcServer)

	grpcListenAddress := fmt.Sprintf(":%d", cfg.BillingServiceGRPCPort)
	grpcListener, err := net.Listen("tcp", grpcListenAddress)
	if err != nil {
		appLogger.Error("Failed to listen for gRPC", "address", grpcListenAddress, "error", err); os.Exit(1)
	}

	g.Go(func() error {
		appLogger.Info("Billing service gRPC server starting", "address", grpcListenAddress)
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, gRPC.ErrServerStopped){
			appLogger.Error("gRPC server failed to serve", "error", err)
			return err
		}
		appLogger.Info("gRPC server shut down gracefully.")
		return nil
	})

	// --- Start HTTP Server for Webhooks ---
	webhookHandler := httpadapter.NewWebhookHandler(billingApp, appLogger) // billingApp implements PaymentWebhookProcessor
	httpRouter := chi.NewRouter()
	httpRouter.Use(chiMiddleware.RequestID)
	httpRouter.Use(chiMiddleware.RealIP)
	httpRouter.Use(chiMiddleware.Recoverer)
	// TODO: Add logging middleware for HTTP requests if desired
	httpRouter.Post("/webhooks/payments", webhookHandler.HandlePaymentWebhook) // Define a clear route

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.BillingServiceHTTPPort),
		Handler: httpRouter,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	g.Go(func() error {
		appLogger.Info("HTTP server for webhooks starting", "address", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("HTTP server ListenAndServe error", "error", err)
			return err
		}
		appLogger.Info("HTTP server shut down gracefully.")
		return nil
	})

	// --- Graceful Shutdown Handling ---
	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)

	g.Go(func() error {
		select {
		case sig := <-stopSignal:
			appLogger.Info("Received termination signal", "signal", sig.String())
			mainCancel() // Start shutdown of all goroutines managed by groupCtx
			return nil
		case <-groupCtx.Done(): // If any goroutine in the group errors out
			return nil // Already handled by the error propagation of errgroup
		}
	})

	// Shutdown goroutine for servers
	g.Go(func() error {
		<-groupCtx.Done() // Wait for mainCancel() or a component failure
		appLogger.Info("Initiating graceful shutdown of servers...")

		shutdownCtx, cancelShutdownTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdownTimeout()

		var shutdownErrors error

		// Shutdown HTTP Server
		appLogger.Info("Attempting graceful shutdown of HTTP server...")
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("HTTP server graceful shutdown failed", "error", err)
			shutdownErrors = errors.Join(shutdownErrors, fmt.Errorf("http shutdown: %w", err))
		} else {
			appLogger.Info("HTTP server shut down successfully.")
		}

		// Shutdown gRPC Server
		appLogger.Info("Attempting graceful shutdown of gRPC server...")
		grpcServer.GracefulStop() // This stops accepting new connections and waits for existing ones
		appLogger.Info("gRPC server has finished GracefulStop.")

		return shutdownErrors
	})


	appLogger.Info("Billing service is ready and running.")
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		// context.Canceled is expected if shutdown is triggered by signal
		appLogger.Error("Service group encountered an error", "error", err)
		// Depending on the error, might want to os.Exit(1) here.
		// If shutdownErrors from the specific shutdown goroutine is critical, handle that too.
	}

	appLogger.Info("Billing service shut down successfully.")
}
