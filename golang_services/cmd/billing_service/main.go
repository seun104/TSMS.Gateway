package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	// "time"

	"github.com/aradsms/golang_services/api/proto/billingservice" // Adjust
	"github.com/aradsms/golang_services/internal/billing_service/adapters/grpc"
	"github.com/aradsms/golang_services/internal/billing_service/app"
	"github.com/aradsms/golang_services/internal/billing_service/repository/postgres"
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/database"
	"github.com/aradsms/golang_services/internal/platform/logger"
	// grpcclient "github.com/aradsms/golang_services/internal/billing_service/adapters/grpc_clients" // For user service client

	gRPC "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg, err := config.Load("../../configs", "config.defaults") // Relative to cmd/billing_service
	if err != nil {
		slog.Error("Failed to load configuration", "error", err); os.Exit(1)
	}
	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("Billing service starting...", "port (grpc)", cfg.BillingServiceGRPCPort, "log_level", cfg.LogLevel)

	dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
	if err != nil {
		appLogger.Error("Failed to connect to PostgreSQL", "error", err); os.Exit(1)
	}
	defer dbPool.Close()
	appLogger.Info("Successfully connected to PostgreSQL")

    // TODO: Initialize UserServiceClient if BillingService needs to call it directly
    // userSvcClient, err := grpcclient.NewUserServiceClient(context.Background(), cfg.UserServiceGRPCClientTarget, appLogger)
    // if err != nil {
    //    appLogger.Error("Failed to connect to user service for billing", "error", err); os.Exit(1)
    // }


	transactionRepo := postgres.NewPgTransactionRepository(dbPool)
	billingApp := app.NewBillingService(transactionRepo, /* userSvcClient (getter), userSvcClient (updater), */ appLogger, cfg)

	grpcServer := gRPC.NewServer()
	billingGRPCServer := grpc.NewBillingGRPCServer(billingApp, appLogger)
	billingservice.RegisterBillingServiceInternalServer(grpcServer, billingGRPCServer)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.BillingServiceGRPCPort)) // Get port from config
	if err != nil {
		appLogger.Error("Failed to listen for gRPC", "port", cfg.BillingServiceGRPCPort, "error", err); os.Exit(1)
	}
	appLogger.Info(fmt.Sprintf("Billing service gRPC server listening on port %d", cfg.BillingServiceGRPCPort))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("gRPC server failed to serve", "error", err)
		}
	}()

	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, syscall.SIGINT, syscall.SIGTERM)
	<-quitChan
	appLogger.Info("Shutdown signal received...")
	grpcServer.GracefulStop()
	appLogger.Info("Billing service shut down successfully.")
}
