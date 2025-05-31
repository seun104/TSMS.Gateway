package main

import (
	"fmt"
	"log/slog"
	"os"

	// Adjust the import path according to your go.mod module name
	"github.com/aradsms/golang_services/internal/platform/config"
	"github.com/aradsms/golang_services/internal/platform/logger"
)

func main() {
	// Load configuration
	// Assuming config.defaults.yaml is in a 'configs' directory relative to where the binary runs,
	// or the path is made absolute/configurable. For this example, let's assume it's in ../../configs
	// In a real deployment, config path would be more robustly determined.
	cfg, err := config.Load("../../configs", "config.defaults") // Path relative to cmd/user_service
	if err != nil {
		slog.Error("Failed to load configuration", "error", err) // Use default slog before custom logger is up
		os.Exit(1)
	}

	// Initialize logger
	appLogger := logger.New(cfg.LogLevel)
	appLogger.Info("User service starting...", "port", cfg.ServerPort, "log_level", cfg.LogLevel)

	// TODO: Initialize Database (PostgreSQL) connection using platform/database
	// dbPool, err := database.NewDBPool(context.Background(), cfg.PostgresDSN)
	// if err != nil {
	//    appLogger.Error("Failed to connect to database", "error", err)
	//    os.Exit(1)
	// }
	// defer dbPool.Close()
	// appLogger.Info("Successfully connected to PostgreSQL database")

	// TODO: Initialize NATS connection using platform/messagebroker
	// natsClient, err := messagebroker.NewNatsClient(cfg.NATSUrl, "user-service", appLogger)
	// if err != nil {
	//    appLogger.Error("Failed to connect to NATS", "error", err)
	//    os.Exit(1)
	// }
	// defer natsClient.Close()
	// appLogger.Info("Successfully connected to NATS")

	// TODO: Setup HTTP or gRPC server
	// For example, using net/http:
	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//     fmt.Fprintf(w, "Hello from User Service!")
	// })
	// appLogger.Info(fmt.Sprintf("Starting server on port %d", cfg.ServerPort))
	// if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ServerPort), nil); err != nil {
	//     appLogger.Error("Failed to start server", "error", err)
	//     os.Exit(1)
	// }

	appLogger.Info("User service running (placeholder - no server started).")
    fmt.Println("User service placeholder is running. See logs for details.")
}
