package grpc_clients

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aradsms/golang_services/api/proto/smsservice" // Adjust
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SMSServiceClient struct {
	client smsservice.SmsQueryServiceInternalClient
	logger *slog.Logger
}

func NewSMSServiceClient(ctx context.Context, targetURL string, logger *slog.Logger) (*SMSServiceClient, error) {
	conn, err := grpc.DialContext(ctx, targetURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SMS service at %s: %w", targetURL, err)
	}
	logger.Info("Connected to SMS service via gRPC", "target", targetURL)
	client := smsservice.NewSmsQueryServiceInternalClient(conn)
	return &SMSServiceClient{client: client, logger: logger.With("client", "sms_service")}, nil
}

func (c *SMSServiceClient) GetOutboxMessageStatus(ctx context.Context, messageID string) (*smsservice.GetOutboxMessageStatusResponse, error) {
	req := &smsservice.GetOutboxMessageStatusRequest{MessageId: messageID}
	return c.client.GetOutboxMessageStatus(ctx, req)
}
