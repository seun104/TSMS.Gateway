package grpc

import (
	"context"
	"errors"
	"log/slog"
	"time" // Added for toProtoTimestamp helper

	"github.com/aradsms/golang_services/api/proto/smsservice" // Adjust to your go.mod path
	"github.com/aradsms/golang_services/internal/core_domain"
	"github.com/aradsms/golang_services/internal/sms_sending_service/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SMSQueryGRPCServer implements the gRPC server for SmsQueryServiceInternal.
type SMSQueryGRPCServer struct {
	smsservice.UnimplementedSmsQueryServiceInternalServer
	outboxRepo repository.OutboxMessageRepository
	logger     *slog.Logger
}

// NewSMSQueryGRPCServer creates a new SMSQueryGRPCServer.
func NewSMSQueryGRPCServer(outboxRepo repository.OutboxMessageRepository, logger *slog.Logger) *SMSQueryGRPCServer {
	return &SMSQueryGRPCServer{
		outboxRepo: outboxRepo,
		logger:     logger.With("component", "sms_query_grpc_server"),
	}
}

func toProtoTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

func strPtrToProtoStr(s *string) string {
    if s == nil {
        return ""
    }
    return *s
}


func (s *SMSQueryGRPCServer) GetOutboxMessageStatus(ctx context.Context, req *smsservice.GetOutboxMessageStatusRequest) (*smsservice.GetOutboxMessageStatusResponse, error) {
	s.logger.InfoContext(ctx, "GetOutboxMessageStatus RPC called", "message_id", req.MessageId)
	if req.MessageId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "message_id is required")
	}

	// TODO: Authorization - Ensure the caller (e.g., public-api-service, identified by its context or a JWT)
	// is allowed to query for this message_id, or that message_id belongs to the authenticated user
	// if user_id was passed in req. This usually means public-api-service passes the authenticated user_id.

	msg, err := s.outboxRepo.GetByID(ctx, req.MessageId)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to get message from repository", "error", err, "message_id", req.MessageId)
		if errors.Is(err, repository.ErrOutboxMessageNotFound) {
			return nil, status.Errorf(codes.NotFound, "message not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve message details")
	}

	protoMsg := &smsservice.OutboxMessageProto{
		Id:                msg.ID,
		UserId:            msg.UserID,
		BatchId:           strPtrToProtoStr(msg.BatchID),
		SenderIdText:      msg.SenderID,
		PrivateNumberId:   strPtrToProtoStr(msg.PrivateNumberID),
		Recipient:         msg.Recipient,
		Content:           msg.Content, // Consider redacting or not sending content for status checks
		Status:            string(msg.Status),
		Segments:          int32(msg.Segments),
		ProviderMessageId: strPtrToProtoStr(msg.ProviderMessageID),
        ProviderStatusCode:strPtrToProtoStr(msg.ProviderStatusCode),
		ErrorMessage:      strPtrToProtoStr(msg.ErrorMessage),
		UserData:          strPtrToProtoStr(msg.UserData),
		ScheduledFor:      toProtoTimestamp(msg.ScheduledFor),
		ProcessedAt:       toProtoTimestamp(msg.ProcessedAt),
		SentAt:            toProtoTimestamp(msg.SentAt),
		DeliveredAt:       toProtoTimestamp(msg.DeliveredAt),
		LastStatusUpdateAt:toProtoTimestamp(msg.LastStatusUpdateAt),
		CreatedAt:         timestamppb.New(msg.CreatedAt), // Assumes CreatedAt is not nil
		UpdatedAt:         timestamppb.New(msg.UpdatedAt), // Assumes UpdatedAt is not nil
        SmsProviderId:     strPtrToProtoStr(msg.SMSProviderID),
        RouteId:           strPtrToProtoStr(msg.RouteID),
	}

	return &smsservice.GetOutboxMessageStatusResponse{Message: protoMsg}, nil
}
