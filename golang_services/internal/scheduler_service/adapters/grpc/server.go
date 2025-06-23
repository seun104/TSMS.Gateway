package grpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/scheduler_service/domain"
	pb "github.com/AradIT/Arad.SMS.Gateway/golang_services/api/proto/schedulerservice"
)

type SchedulerServer struct {
	pb.UnimplementedSchedulerServiceServer
	repo   domain.ScheduledJobRepository
	logger *slog.Logger
}

func NewSchedulerServer(repo domain.ScheduledJobRepository, logger *slog.Logger) *SchedulerServer {
	return &SchedulerServer{repo: repo, logger: logger}
}

func (s *SchedulerServer) CreateScheduledMessage(ctx context.Context, req *pb.CreateScheduledMessageRequest) (*pb.ScheduledMessage, error) {
	s.logger.InfoContext(ctx, "CreateScheduledMessage RPC called")

	if req.GetJobType() == "" {
		s.logger.WarnContext(ctx, "CreateScheduledMessage validation error: JobType is required")
		return nil, status.Errorf(codes.InvalidArgument, "JobType is required")
	}
	if req.GetScheduleTime() == nil {
		s.logger.WarnContext(ctx, "CreateScheduledMessage validation error: ScheduleTime is required")
		return nil, status.Errorf(codes.InvalidArgument, "ScheduleTime is required")
	}
	if req.GetScheduleTime().AsTime().Before(time.Now()) {
		s.logger.WarnContext(ctx, "CreateScheduledMessage validation error: ScheduleTime must be in the future")
		return nil, status.Errorf(codes.InvalidArgument, "ScheduleTime must be in the future")
	}

	var userID uuid.NullUUID
	if req.GetUserId() != "" {
		parsedUUID, err := uuid.Parse(req.GetUserId())
		if err != nil {
			s.logger.WarnContext(ctx, "CreateScheduledMessage validation error: Invalid UserID format", "user_id", req.GetUserId(), "error", err)
			return nil, status.Errorf(codes.InvalidArgument, "Invalid UserID format: %v", err)
		}
		userID = uuid.NullUUID{UUID: parsedUUID, Valid: true}
	}

	payloadBytes, err := json.Marshal(req.GetPayload())
	if err != nil {
		s.logger.ErrorContext(ctx, "CreateScheduledMessage: failed to marshal payload", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to marshal payload: %v", err)
	}

	job := &domain.ScheduledJob{
		ID:            uuid.New(),
		JobType:       req.GetJobType(),
		Payload:       json.RawMessage(payloadBytes),
		ScheduledAt:   req.GetScheduleTime().AsTime(),
		Status:        string(domain.StatusPending), // Use domain.StatusPending
		UserID:        userID,
		RetryCount:    0,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	err = s.repo.Create(ctx, job)
	if err != nil {
		s.logger.ErrorContext(ctx, "CreateScheduledMessage: failed to create scheduled message in repo", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create scheduled message: %v", err)
	}

	s.logger.InfoContext(ctx, "Scheduled message created successfully", "job_id", job.ID.String())
	return scheduledJobToProto(job), nil
}

func (s *SchedulerServer) GetScheduledMessage(ctx context.Context, req *pb.GetScheduledMessageRequest) (*pb.ScheduledMessage, error) {
	s.logger.InfoContext(ctx, "GetScheduledMessage RPC called", "job_id", req.GetId())
	if req.GetId() == "" {
		s.logger.WarnContext(ctx, "GetScheduledMessage validation error: ID is required")
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	jobID, err := uuid.Parse(req.GetId())
	if err != nil {
		s.logger.WarnContext(ctx, "GetScheduledMessage validation error: Invalid ID format", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "Invalid ID format: %v", err)
	}

	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.WarnContext(ctx, "GetScheduledMessage: job not found", "job_id", req.GetId())
			return nil, status.Errorf(codes.NotFound, "scheduled message not found")
		}
		s.logger.ErrorContext(ctx, "GetScheduledMessage: failed to get scheduled message from repo", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.Internal, "failed to get scheduled message: %v", err)
	}
	return scheduledJobToProto(job), nil
}

func (s *SchedulerServer) ListScheduledMessages(ctx context.Context, req *pb.ListScheduledMessagesRequest) (*pb.ListScheduledMessagesResponse, error) {
	s.logger.InfoContext(ctx, "ListScheduledMessages RPC called", "page_size", req.GetPageSize(), "page_number", req.GetPageNumber(), "user_id", req.GetUserId(), "status", req.GetStatus(), "job_type", req.GetJobType())

	pageSize := int(req.GetPageSize())
	pageNumber := int(req.GetPageNumber())
	if pageSize <= 0 {
		pageSize = 10 // Default page size
	}
	if pageNumber <= 0 {
		pageNumber = 1 // Default page number
	}

	var userIDFilter uuid.NullUUID
	if req.GetUserId() != "" {
		parsedUUID, err := uuid.Parse(req.GetUserId())
		if err != nil {
			s.logger.WarnContext(ctx, "ListScheduledMessages validation error: Invalid UserID format", "user_id", req.GetUserId(), "error", err)
			return nil, status.Errorf(codes.InvalidArgument, "Invalid UserID filter format: %v", err)
		}
		userIDFilter = uuid.NullUUID{UUID: parsedUUID, Valid: true}
	}

	jobs, totalCount, err := s.repo.List(ctx, userIDFilter, req.GetStatus(), req.GetJobType(), pageSize, pageNumber)
	if err != nil {
		s.logger.ErrorContext(ctx, "ListScheduledMessages: failed to list messages from repo", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list scheduled messages: %v", err)
	}

	protoMessages := make([]*pb.ScheduledMessage, len(jobs))
	for i, job := range jobs {
		protoMessages[i] = scheduledJobToProto(job)
	}

	return &pb.ListScheduledMessagesResponse{
		Messages:   protoMessages,
		TotalCount: int32(totalCount),
	}, nil
}

func (s *SchedulerServer) UpdateScheduledMessage(ctx context.Context, req *pb.UpdateScheduledMessageRequest) (*pb.ScheduledMessage, error) {
	s.logger.InfoContext(ctx, "UpdateScheduledMessage RPC called", "job_id", req.GetId())
	if req.GetId() == "" {
		s.logger.WarnContext(ctx, "UpdateScheduledMessage validation error: ID is required")
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	jobID, err := uuid.Parse(req.GetId())
	if err != nil {
		s.logger.WarnContext(ctx, "UpdateScheduledMessage validation error: Invalid ID format", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "Invalid ID format: %v", err)
	}

	existingJob, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.WarnContext(ctx, "UpdateScheduledMessage: job not found", "job_id", req.GetId())
			return nil, status.Errorf(codes.NotFound, "scheduled message not found")
		}
		s.logger.ErrorContext(ctx, "UpdateScheduledMessage: failed to get existing message from repo", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.Internal, "failed to retrieve scheduled message for update: %v", err)
	}

	// Update fields if provided in the request
	if req.GetPayload() != nil { // Check for nil map
		payloadBytes, err := json.Marshal(req.GetPayload())
		if err != nil {
			s.logger.ErrorContext(ctx, "UpdateScheduledMessage: failed to marshal payload", "job_id", req.GetId(), "error", err)
			return nil, status.Errorf(codes.Internal, "failed to marshal payload: %v", err)
		}
		existingJob.Payload = json.RawMessage(payloadBytes)
	}

	if req.GetScheduleTime() != nil {
		if req.GetScheduleTime().AsTime().Before(time.Now().Add(-5 * time.Minute)) && existingJob.Status == string(domain.StatusPending) { // Allow minor past time for updates if already past
             s.logger.WarnContext(ctx, "UpdateScheduledMessage validation error: ScheduleTime must be in the future for pending jobs", "job_id", req.GetId(), "schedule_time", req.GetScheduleTime().AsTime())
             return nil, status.Errorf(codes.InvalidArgument, "ScheduleTime must be in the future for pending jobs")
        }
		existingJob.ScheduledAt = req.GetScheduleTime().AsTime()
	}

	if req.GetStatus() != "" {
		// Basic validation for status transitions can be added here if needed
		// For example, cannot move from "completed" back to "pending"
		if !isValidStatusTransition(existingJob.Status, req.GetStatus()) {
			s.logger.WarnContext(ctx, "UpdateScheduledMessage validation error: Invalid status transition", "job_id", req.GetId(), "from_status", existingJob.Status, "to_status", req.GetStatus())
			return nil, status.Errorf(codes.InvalidArgument, "Invalid status transition from %s to %s", existingJob.Status, req.GetStatus())
		}
		existingJob.Status = req.GetStatus()
	}

	existingJob.UpdatedAt = time.Now().UTC()

	err = s.repo.Update(ctx, existingJob)
	if err != nil {
		s.logger.ErrorContext(ctx, "UpdateScheduledMessage: failed to update message in repo", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update scheduled message: %v", err)
	}

	s.logger.InfoContext(ctx, "Scheduled message updated successfully", "job_id", existingJob.ID.String())
	return scheduledJobToProto(existingJob), nil
}

func (s *SchedulerServer) DeleteScheduledMessage(ctx context.Context, req *pb.DeleteScheduledMessageRequest) (*emptypb.Empty, error) {
	s.logger.InfoContext(ctx, "DeleteScheduledMessage RPC called", "job_id", req.GetId())
	if req.GetId() == "" {
		s.logger.WarnContext(ctx, "DeleteScheduledMessage validation error: ID is required")
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	jobID, err := uuid.Parse(req.GetId())
	if err != nil {
		s.logger.WarnContext(ctx, "DeleteScheduledMessage validation error: Invalid ID format", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "Invalid ID format: %v", err)
	}

	// Optional: Check if job can be deleted (e.g., not in "processing" state)
	// currentJob, err := s.repo.GetByID(ctx, jobID)
	// if err == nil && currentJob.Status == string(domain.StatusProcessing) {
	//    s.logger.WarnContext(ctx, "DeleteScheduledMessage: cannot delete job in processing state", "job_id", req.GetId())
	// 	  return nil, status.Errorf(codes.FailedPrecondition, "cannot delete a job that is currently processing")
	// }


	err = s.repo.Delete(ctx, jobID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.WarnContext(ctx, "DeleteScheduledMessage: job not found", "job_id", req.GetId())
			return nil, status.Errorf(codes.NotFound, "scheduled message not found")
		}
		s.logger.ErrorContext(ctx, "DeleteScheduledMessage: failed to delete message from repo", "job_id", req.GetId(), "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete scheduled message: %v", err)
	}

	s.logger.InfoContext(ctx, "Scheduled message deleted successfully", "job_id", req.GetId())
	return &emptypb.Empty{}, nil
}

func scheduledJobToProto(job *domain.ScheduledJob) *pb.ScheduledMessage {
	var payloadMap map[string]string
	if len(job.Payload) > 0 { // Check if payload is not empty
		err := json.Unmarshal(job.Payload, &payloadMap)
		if err != nil {
			// Log or handle error, perhaps return an error in actual service
			// For now, we'll leave payloadMap as nil or empty if unmarshalling fails
		}
	}

	userIDStr := ""
	if job.UserID.Valid {
		userIDStr = job.UserID.UUID.String()
	}

	// Handle nil sql.NullString for Error field
	// var errorMsg string
	// if job.Error.Valid {
	// 	errorMsg = job.Error.String
	// }
    // The domain.ScheduledJob.Error is json.RawMessage, not sql.NullString.
    // It should be mapped to a string field in proto if needed, or handled as part of payload.
    // For now, the proto does not have an error message field.

	return &pb.ScheduledMessage{
		Id:           job.ID.String(),
		JobType:      job.JobType,
		Payload:      payloadMap,
		ScheduleTime: timestamppb.New(job.ScheduledAt),
		CreatedAt:    timestamppb.New(job.CreatedAt),
		UpdatedAt:    timestamppb.New(job.UpdatedAt),
		Status:       job.Status,
		UserId:       userIDStr,
	}
}

// Placeholder for status transition validation logic
func isValidStatusTransition(currentStatus string, newStatus string) bool {
	// Example: Prevent moving from a terminal state back to a pending state.
	if (currentStatus == string(domain.StatusCompleted) || currentStatus == string(domain.StatusFailed)) &&
	   (newStatus == string(domain.StatusPending) || newStatus == string(domain.StatusProcessing) || newStatus == string(domain.StatusRetry)) {
		return false
	}
	// Allow most other transitions. More specific rules can be added.
	return true
}

// Helper function to convert proto to domain.ScheduledJob for updates (if needed)
// func protoToScheduledJobUpdate(req *pb.UpdateScheduledMessageRequest, existingJob *domain.ScheduledJob) *domain.ScheduledJob {
//   // ... implementation ...
//   return existingJob
// }

// errors is not imported in the original example, let's add it.
import "errors"
