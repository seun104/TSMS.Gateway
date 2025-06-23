package grpc_clients

import (
	"context"
	"fmt"
	"log/slog" // Or your platform logger

	"github.com/aradsms/golang_services/api/proto/userservice" // Adjust to your go.mod path
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // For local dev; use secure creds in prod
)

// UserServiceClient wraps the gRPC client for the user service.
type UserServiceClient struct {
	client userservice.AuthServiceInternalClient
	logger *slog.Logger
}

// NewUserServiceClient creates a new gRPC client for the user service.
func NewUserServiceClient(ctx context.Context, targetURL string, logger *slog.Logger) (*UserServiceClient, error) {
	// In production, use grpc.WithTransportCredentials(creds)
	// For local development, insecure credentials are fine.
	// TODO: Add mTLS or other secure credentials for production.
	conn, err := grpc.DialContext(ctx, targetURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Optional: block until connection is up or fails
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to user service at %s: %w", targetURL, err)
	}
	logger.Info("Connected to user service via gRPC", "target", targetURL)

	client := userservice.NewAuthServiceInternalClient(conn)
	return &UserServiceClient{client: client, logger: logger.With("client", "user_service")}, nil
}

// ValidateToken calls the user service to validate a token.
func (c *UserServiceClient) ValidateToken(ctx context.Context, token string) (*userservice.ValidatedUserResponse, error) {
	req := &userservice.ValidateTokenRequest{Token: token}
	res, err := c.client.ValidateToken(ctx, req)
	if err != nil {
		c.logger.WarnContext(ctx, "ValidateToken RPC call failed", "error", err)
		// Do not wrap gRPC specific errors directly to higher layers if not needed.
		// Convert to application-specific errors if necessary.
		return nil, err // Or a more generic error
	}
	return res, nil
}

// GetUserPermissions calls the user service to get permissions.
func (c *UserServiceClient) GetUserPermissions(ctx context.Context, userID string) (*userservice.GetUserPermissionsResponse, error) {
    req := &userservice.GetUserPermissionsRequest{UserId: userID}
    res, err := c.client.GetUserPermissions(ctx, req)
    if err != nil {
        c.logger.WarnContext(ctx, "GetUserPermissions RPC call failed", "error", err, "userID", userID)
        return nil, err
    }
    return res, nil
}
