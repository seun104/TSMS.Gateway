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

	// Prometheus
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
)

const (
	serviceName         = "billing-service"
	defaultMetricsPort  = 9093               // Default port for Prometheus metrics
	shutdownTimeout     = 15 * time.Second // Shared timeout for graceful shutdown
)

// httpLogger is a middleware that logs HTTP requests using slog.
func httpLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			requestID := chiMiddleware.GetReqID(r.Context())
			remoteIP := chiMiddleware.GetRealIP(r.Context())

			next.ServeHTTP(ww, r)

			duration := time.Since(start)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "HTTP request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status_code", ww.Status()),
				slog.Int64("duration_ms", duration.Milliseconds()),
				slog.String("request_id", requestID),
				slog.String("remote_ip", remoteIP),
			)
		}
		return http.HandlerFunc(fn)
	}
}

func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	cfg, err := config.Load(serviceName) // Use serviceName for config loading
	if err != nil {
		slog.Error("Failed to load configuration", "error", err); os.Exit(1) // Use default slog if appLogger not yet init
	}
	appLogger := logger.New(cfg.LogLevel) // Initialize early
	appLogger = appLogger.With("service", serviceName) // Add service context to logger

	// Determine metrics port
	metricsPort := cfg.BillingServiceMetricsPort
	if metricsPort == 0 {
		metricsPort = defaultMetricsPort
		appLogger.Info("Billing service metrics port not configured, using default", "port", metricsPort)
	}
	appLogger.Info("Billing service starting...",
		"grpc_port", cfg.BillingServiceGRPCPort,
		"http_port", cfg.BillingServiceHTTPPort,
		"metrics_port", metricsPort,
		"log_level", cfg.LogLevel,
	)

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
	// Initialize Prometheus gRPC metrics
	grpcMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(),
	)
	if err := prometheus.DefaultRegisterer.Register(grpcMetrics); err != nil {
		// Handle potential error, e.g. if metrics are already registered by another instance in same process.
		// For this service, it's unlikely unless there's a specific test setup causing it.
		// If it's a "already registered" error, we might choose to log it and continue,
		// or treat it as fatal. Let's log and continue for now.
		appLogger.Warn("Failed to register gRPC Prometheus metrics", "error", err)
	}


	grpcServer := gRPC.NewServer(
		gRPC.ChainUnaryInterceptor(grpcMetrics.UnaryServerInterceptor()),
		gRPC.ChainStreamInterceptor(grpcMetrics.StreamServerInterceptor()),
	)
	billingGRPCServer := grpcadapter.NewBillingGRPCServer(billingApp, appLogger)
	billingservice.RegisterBillingServiceInternalServer(grpcServer, billingGRPCServer)
	reflection.Register(grpcServer)
	grpcMetrics.InitializeMetrics(grpcServer) // Initialize metrics for the server

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
	httpRouter.Use(chiMiddleware.Recoverer) // Recover from panics
	httpRouter.Use(httpLogger(appLogger))   // Log HTTP requests using our new middleware
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

	// --- Start Metrics HTTP Server ---
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", metricsPort),
		Handler: metricsMux,
	}

	g.Go(func() error {
		appLogger.Info("Metrics HTTP server starting", "address", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("Metrics HTTP server ListenAndServe error", "error", err)
			return err
		}
		appLogger.Info("Metrics HTTP server shut down gracefully.")
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

		// Shutdown Metrics Server
		appLogger.Info("Attempting graceful shutdown of Metrics HTTP server...")
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("Metrics HTTP server graceful shutdown failed", "error", err)
			shutdownErrors = errors.Join(shutdownErrors, fmt.Errorf("metrics http shutdown: %w", err))
		} else {
			appLogger.Info("Metrics HTTP server shut down successfully.")
		}

		// Shutdown Webhook HTTP Server
		appLogger.Info("Attempting graceful shutdown of Webhook HTTP server...")
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("Webhook HTTP server graceful shutdown failed", "error", err)
			shutdownErrors = errors.Join(shutdownErrors, fmt.Errorf("webhook http shutdown: %w", err))
		} else {
			appLogger.Info("Webhook HTTP server shut down successfully.")
		}

		// Shutdown gRPC Server
		appLogger.Info("Attempting graceful shutdown of gRPC server...")
		grpcServer.GracefulStop() // This stops accepting new connections and waits for existing ones
		appLogger.Info("gRPC server has finished GracefulStop.")

		return shutdownErrors
	})


	appLogger.Info("Billing service is ready and running.")
	if err := g.Wait(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, gRPC.ErrServerStopped) {
			appLogger.Error("Service group encountered an error during run/shutdown", "error", err)
		}
	}

	appLogger.Info("Billing service shut down successfully.")
}
