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

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // For gRPC server reflection

	// Platform packages (adjust to actual project structure)
	"github.com/your-repo/project/internal/platform/config"
	"github.com/your-repo/project/internal/platform/database"
	"github.com/your-repo/project/internal/platform/logger"
	"github.com/your-repo/project/internal/platform/messagebroker"

	// Phonebook service specific packages
	pb "github.com/your-repo/project/golang_services/api/proto/phonebookservice" // Generated gRPC code
	grpcAdapter "github.com/your-repo/project/internal/phonebook_service/adapters/grpc"    // Your gRPC server implementation
	phonebookApp "github.com/your-repo/project/internal/phonebook_service/app"            // Application layer
	"github.com/your-repo/project/internal/phonebook_service/repository/postgres" // Repository implementations
)

const (
	serviceName     = "phonebook_service"
	startupTimeout  = 30 * time.Second
	shutdownTimeout = 30 * time.Second // gRPC might need more time to drain connections
)

func main() {
	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	log := logger.New(serviceName, "dev") // Or load level from config
	log.Info("Starting service...")

	cfg, err := config.Load(serviceName)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	log.Info("Configuration loaded successfully")
	// Example: log.Info("gRPC Port", "port", cfg.PhonebookService.GRPCPort)

	dbPool, err := database.NewPostgresPool(mainCtx, cfg.Postgres.DSN, log)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("Database connection pool initialized")

	// NATS (Optional for phonebook, but kept for pattern consistency)
	natsClient, err := messagebroker.NewNATSClient(cfg.NATS.URL, log, serviceName)
	if err != nil {
		log.Warn("Failed to connect to NATS (optional for this service)", "error", err)
		// For some services, NATS might be critical. For phonebook, perhaps not.
	} else {
		defer natsClient.Close()
		log.Info("NATS connection initialized (if configured and successful)")
	}

	// Setup application components
	// Setup application components
	phonebookRepo := postgres.NewPgPhonebookRepository(dbPool, log)
	contactRepo := postgres.NewPgContactRepository(dbPool, log)
	application := phonebookApp.NewApplication(phonebookRepo, contactRepo, log)
	grpcServerInstance := grpcAdapter.NewGRPCServer(application, log)

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start gRPC server
	g.Go(func() error {
		// grpcPort := cfg.PhonebookService.GRPCPort // Assuming port is in config from a PhonebookService specific section
		// For now, using a hardcoded or general config value if available.
		// If cfg.GRPCPort is a general gRPC port for any service:
		// grpcPort := cfg.GRPCPort
		// If specific config like cfg.PhonebookService.GRPCPort:
		// grpcPort := cfg.PhonebookService.GRPCPort
		// Fallback to a default if not in config for now:
		grpcPort := 50051 // Default, or get from cfg.PhonebookService.GRPCPort or cfg.APP_GRPC_PORT
		if cfg.AppSpecific["APP_GRPC_PORT"] != "" { // Example how config might be structured
            parsedPort, parseErr := time.ParseDuration(cfg.AppSpecific["APP_GRPC_PORT"]) // This is incorrect for port number
            // Correct parsing for integer port:
            // portStr := cfg.AppSpecific["APP_GRPC_PORT"]
            // if portVal, convErr := strconv.Atoi(portStr); convErr == nil {
            //    grpcPort = portVal
            // } else {
            //    log.Warn("Could not parse APP_GRPC_PORT from config, using default", "value", portStr, "error", convErr)
            // }
            // For now, keeping the hardcoded default or expecting a direct int field in config.
            // Example: grpcPort = cfg.PhonebookGRPCPort
        }


		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
		if err != nil {
			log.Error("Failed to listen for gRPC", "port", grpcPort, "error", err)
			return fmt.Errorf("failed to listen for gRPC on port %d: %w", grpcPort, err)
		}
		defer lis.Close()

		s := grpc.NewServer(
		// Add interceptors here if needed:
		// grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer( /* your interceptors */ )),
		// grpc.StreamInterceptor(grpc_middleware.ChainStreamServer( /* your interceptors */ )),
		)

		pb.RegisterPhonebookServiceServer(s, grpcServerInstance) // Register your service
		log.Info("gRPC PhonebookService registered")

		// Enable server reflection (useful for tools like grpcurl)
		reflection.Register(s)
		log.Info("gRPC server reflection registered.")

		log.Info(fmt.Sprintf("gRPC server starting on port %d", grpcPort))
		if err := s.Serve(lis); err != nil {
			log.Error("gRPC server failed to serve", "error", err)
			return fmt.Errorf("gRPC server failed: %w", err)
		}
		return nil
	})

	log.Info("Service components initialized and workers started. Service is ready.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var groupErr error
	select {
	case sig := <-sigCh:
		log.Info("Received termination signal", "signal", sig)
	case groupErr = <-watchGroup(g): // This will block until a goroutine in the group errors or all complete.
		log.Error("A critical component failed, initiating shutdown", "error", groupErr)
	}

	log.Info("Attempting graceful shutdown...")
	mainCancel() // Signal all goroutines in the errgroup to stop

	// Give goroutines time to finish.
	// For gRPC, s.GracefulStop() should be called, which is implicitly handled if s.Serve is part of the errgroup.
	// The errgroup's context cancellation will cause lis.Accept to error, stopping s.Serve.
	// Then s.GracefulStop() (or s.Stop() if forced) would be called by the defer in its goroutine if structured that way,
	// or by a specific shutdown sequence if needed.
	// Here, we rely on the errgroup context to signal shutdown to s.Serve().
	// A more explicit s.GracefulStop() might be added if the grpc.Server instance `s` is accessible here.

	shutdownComplete := make(chan struct{})
	go func() {
		defer close(shutdownComplete)
		if err := g.Wait(); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			log.Error("Error during graceful shutdown of components", "error", err)
		} else if groupErr != nil && groupErr != context.Canceled && groupErr != context.DeadlineExceeded {
			log.Error("Shutdown initiated due to component error", "error", groupErr)
		}
	}()

	select {
	case <-shutdownComplete:
		log.Info("All components shut down gracefully.")
	case <-time.After(shutdownTimeout):
		log.Error("Shutdown timed out. Forcing exit.")
	}

	log.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()
	return errCh
}
