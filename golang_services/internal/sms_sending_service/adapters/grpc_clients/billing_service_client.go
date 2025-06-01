package grpc_clients

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aradsms/golang_services/api/proto/billingservice" // Adjust to your go.mod path
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // For local dev
)

// BillingServiceClient wraps the gRPC client for the billing service.
type BillingServiceClient struct {
	client billingservice.BillingServiceInternalClient
	logger *slog.Logger
}

// NewBillingServiceClient creates a new gRPC client for the billing service.
func NewBillingServiceClient(ctx context.Context, targetURL string, logger *slog.Logger) (*BillingServiceClient, error) {
	conn, err := grpc.DialContext(ctx, targetURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to billing service at %s: %w", targetURL, err)
	}
	logger.Info("Connected to billing service via gRPC", "target", targetURL)

	client := billingservice.NewBillingServiceInternalClient(conn)
	return &BillingServiceClient{client: client, logger: logger.With("client", "billing_service")}, nil
}

// CheckAndDeductCredit calls the billing service.
func (c *BillingServiceClient) CheckAndDeductCredit(ctx context.Context, req *billingservice.CheckAndDeductCreditRequest) (*billingservice.CheckAndDeductCreditResponse, error) {
	res, err := c.client.CheckAndDeductCredit(ctx, req)
	if err != nil {
		c.logger.ErrorContext(ctx, "CheckAndDeductCredit RPC call failed", "error", err, "userID", req.UserId)
		return nil, err
	}
	return res, nil
}
