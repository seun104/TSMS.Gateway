package grpc_clients

import (
	"fmt"
	"log/slog" // It's good practice to include a logger

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/platform/config" // Assuming this is the public-api-service's config
	pb "github.com/AradIT/Arad.SMS.Gateway/golang_services/api/proto/schedulerservice"
)

type SchedulerServiceClient struct {
	client pb.SchedulerServiceClient
	conn   *grpc.ClientConn
	logger *slog.Logger
}

// NewSchedulerServiceClient creates a new gRPC client for the SchedulerService.
// It expects the target address string (e.g., "localhost:50053") and a logger.
func NewSchedulerServiceClient(target string, logger *slog.Logger) (*SchedulerServiceClient, error) {
	if logger == nil {
		// Fallback to a default logger if none is provided, though it's better to ensure one is passed.
		logger = slog.Default()
	}

	if target == "" {
		logger.Error("SchedulerService GRPC client target address is required")
		return nil, fmt.Errorf("scheduler service grpc target address is required")
	}

	logger.Info("Connecting to SchedulerService gRPC server", "address", target)

	// For simplicity in dev, using insecure. In prod, use TLS.
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("Failed to connect to scheduler service", "address", target, "error", err)
		return nil, fmt.Errorf("failed to connect to scheduler service at %s: %w", target, err)
	}
	logger.Info("Successfully connected to SchedulerService gRPC server", "address", target)

	client := pb.NewSchedulerServiceClient(conn)
	return &SchedulerServiceClient{client: client, conn: conn, logger: logger}, nil
}

func (c *SchedulerServiceClient) GetClient() pb.SchedulerServiceClient {
	return c.client
}

func (c *SchedulerServiceClient) Close() error {
	if c.conn != nil {
		c.logger.Info("Closing SchedulerService gRPC client connection")
		return c.conn.Close()
	}
	return nil
}
