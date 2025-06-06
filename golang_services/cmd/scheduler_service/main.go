package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fmt"
	"net"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"


	// Platform packages
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/platform/config"
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/platform/database"
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/platform/logger"
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/platform/messagebroker"

	// Scheduler service specific packages
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/scheduler_service/app"
	grpcadapter "github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/scheduler_service/adapters/grpc"
	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/scheduler_service/repository/postgres"
	pb "github.com/AradIT/Arad.SMS.Gateway/golang_services/api/proto/schedulerservice"
)

const (
	serviceName     = "scheduler-service" // Consistent naming with other services
	startupTimeout  = 30 * time.Second    // TODO: Consider moving to config
	shutdownTimeout = 10 * time.Second    // TODO: Consider moving to config
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
	// Example: log.Info("NATS URL", "url", cfg.NATS.URL)
	// Example: log.Info("Postgres DSN", "dsn", cfg.Postgres.DSN)

	dbPool, err := database.NewPostgresPool(mainCtx, cfg.Postgres.DSN, log)
	if err != nil {
		log.Error("Failed to initialize database connection pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("Database connection pool initialized")

	natsClient, err := messagebroker.NewNATSClient(cfg.NATS.URL, log, serviceName)
	if err != nil {
		log.Error("Failed to connect to NATS", "error", err) // NATS is critical for publishing jobs
		os.Exit(1)
	}
	defer natsClient.Close()
	log.Info("NATS connection initialized")

	// Setup application components
	// Setup application components
	scheduledJobRepo := postgres.NewPgScheduledJobRepository(dbPool, log)

	pollerCfg := app.PollerConfig{
		PollingInterval: cfg.SchedulerPollingInterval,
		JobBatchSize:    cfg.SchedulerJobBatchSize,
		MaxRetry:        cfg.SchedulerMaxRetry,
	}
	jobPoller := app.NewJobPoller(scheduledJobRepo, natsClient, log, pollerCfg)

	// Initialize gRPC server
	schedulerServer := grpcadapter.NewSchedulerServer(scheduledJobRepo, log)
	grpcServer := grpc.NewServer()
	pb.RegisterSchedulerServiceServer(grpcServer, schedulerServer)
	reflection.Register(grpcServer) // Optional: for gRPC reflection

	g, groupCtx := errgroup.WithContext(mainCtx)

	// Start the main scheduling loop/ticker
	g.Go(func() error {
		log.Info("Starting scheduler job poller worker...", "polling_interval", pollerCfg.PollingInterval)
		ticker := time.NewTicker(pollerCfg.PollingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.InfoContext(groupCtx, "Job poller tick")
				processed, pollErr := jobPoller.PollAndProcessJobs(groupCtx)
				if pollErr != nil {
					// This is a critical error from the poller itself (e.g. DB connection issue during acquire)
					log.ErrorContext(groupCtx, "Job poller encountered a critical error, stopping.", "error", pollErr)
					return pollErr // Stop the goroutine, which will stop the service via errgroup
				}
				if processed > 0 {
					log.InfoContext(groupCtx, "Job poller processed jobs in this tick", "count", processed)
				}
			case <-groupCtx.Done():
				log.InfoContext(groupCtx, "Job poller worker stopping due to group context done.", "error", groupCtx.Err())
				return groupCtx.Err()
			}
		}
	})

	// Start gRPC server
	g.Go(func() error {
		grpcListenAddress := fmt.Sprintf(":%d", cfg.GRPC.Port) // Assuming cfg.GRPC.Port is available e.g. 50053
		log.Info("Starting gRPC server...", "address", grpcListenAddress)
		lis, err := net.Listen("tcp", grpcListenAddress)
		if err != nil {
			log.Error("Failed to listen for gRPC", "error", err)
			return err
		}
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Error("gRPC server failed", "error", err)
			return err
		}
		log.Info("gRPC server stopped gracefully.")
		return nil
	})

	// Goroutine to handle graceful shutdown of gRPC server
	g.Go(func() error {
		<-groupCtx.Done() // Wait for stop signal
		log.Info("Initiating gRPC server graceful shutdown...")
		grpcServer.GracefulStop()
		log.Info("gRPC server has been shut down.")
		return nil
	})


	log.Info("Service components initialized and workers started. Service is ready.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var groupErr error
	select {
	case sig := <-sigCh:
		log.Info("Received termination signal", "signal", sig)
	case groupErr = <-watchGroup(g): // This channel receives from g.Wait()
		if groupErr != nil { // Should not be nil if this path is taken
			log.Error("A critical component failed, initiating shutdown", "error", groupErr)
		} else {
			log.Info("All components finished without error, initiating shutdown.") // Should typically not happen unless all goroutines exit cleanly on their own
		}
	}

	log.Info("Attempting graceful shutdown...")
	mainCancel() // Signal all goroutines managed by groupCtx to stop

	// Create a new context for the final Wait, as groupCtx might be canceled
	shutdownCtx, shutdownCancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancelTimeout()

	// Wait for all errgroup goroutines to finish
	waitErr := g.Wait()

	if waitErr != nil && !errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, context.DeadlineExceeded) {
		log.Error("Error during graceful shutdown of components", "error", waitErr)
	} else if groupErr != nil && !errors.Is(groupErr, context.Canceled) && !errors.Is(groupErr, context.DeadlineExceeded) {
		// This logs the error that initially triggered the shutdown, if it wasn't a signal
		log.Error("Shutdown initiated due to component error", "error", groupErr)
	}


	log.Info("Service shutdown complete.")
}

// watchGroup is a helper to monitor an errgroup for early exit.
// It returns a channel that will receive the error from g.Wait().
func watchGroup(g *errgroup.Group) <-chan error {
	errCh := make(chan error, 1) // Buffer of 1 in case send happens before receive
	go func() {
		// g.Wait() blocks until all goroutines in the group have returned,
		// then returns the first non-nil error (if any) from them.
		errCh <- g.Wait()
		close(errCh)
	}()
	return errCh
}

// Ensure errors is imported for grpc.ErrServerStopped and context errors
import "errors"
