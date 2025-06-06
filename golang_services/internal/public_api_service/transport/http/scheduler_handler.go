package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AradIT/Arad.SMS.Gateway/golang_services/internal/public_api_service/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"

	pb "github.com/AradIT/Arad.SMS.Gateway/golang_services/api/proto/schedulerservice"
)

type SchedulerHandler struct {
	schedulerClient pb.SchedulerServiceClient
	logger          *slog.Logger
	validate        *validator.Validate
}

func NewSchedulerHandler(client pb.SchedulerServiceClient, logger *slog.Logger, validate *validator.Validate) *SchedulerHandler {
	return &SchedulerHandler{
		schedulerClient: client,
		logger:          logger,
		validate:        validate,
	}
}

// Helper to map gRPC errors to HTTP status codes
func mapGRPCErrorToHTTPStatus(w http.ResponseWriter, logger *slog.Logger, err error, operation string, resourceID string) {
	if err == nil {
		return
	}
	st, ok := status.FromError(err)
	if !ok {
		logger.Error(fmt.Sprintf("%s failed: unable to parse gRPC error", operation), "resource_id", resourceID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logEntry := logger.With("grpc_code", st.Code().String(), "grpc_message", st.Message(), "operation", operation, "resource_id", resourceID)

	switch st.Code() {
	case codes.InvalidArgument:
		logEntry.Warn("InvalidArgument gRPC error")
		http.Error(w, fmt.Sprintf("Invalid request: %s", st.Message()), http.StatusBadRequest)
	case codes.NotFound:
		logEntry.Warn("NotFound gRPC error")
		http.Error(w, fmt.Sprintf("Resource not found: %s", st.Message()), http.StatusNotFound)
	case codes.AlreadyExists:
		logEntry.Warn("AlreadyExists gRPC error")
		http.Error(w, fmt.Sprintf("Resource already exists: %s", st.Message()), http.StatusConflict)
	case codes.PermissionDenied:
		logEntry.Warn("PermissionDenied gRPC error")
		http.Error(w, "Permission denied", http.StatusForbidden)
	case codes.Unauthenticated:
		logEntry.Warn("Unauthenticated gRPC error")
		http.Error(w, "Unauthenticated", http.StatusUnauthorized)
	default:
		logEntry.Error("Unhandled gRPC error")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *SchedulerHandler) CreateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var reqDTO CreateScheduledMessageRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.logger.WarnContext(ctx, "Failed to decode request body for CreateScheduledMessage", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		h.logger.WarnContext(ctx, "Validation failed for CreateScheduledMessage", "error", err)
		http.Error(w, fmt.Sprintf("Validation error: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if reqDTO.ScheduleTime.Before(time.Now().Add(10*time.Second)) { // Ensure schedule time is reasonably in future
		h.logger.WarnContext(ctx, "Validation failed for CreateScheduledMessage: ScheduleTime must be in the future.", "schedule_time", reqDTO.ScheduleTime)
		http.Error(w, "ScheduleTime must be at least 10 seconds in the future.", http.StatusBadRequest)
		return
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok {
		h.logger.ErrorContext(ctx, "AuthenticatedUser not found in context for CreateScheduledMessage")
		http.Error(w, "User authentication details not found", http.StatusUnauthorized)
		return
	}

	targetUserID := authUser.ID // Default to authenticated user's ID
	if reqDTO.UserID != "" {
		if reqDTO.UserID != authUser.ID && !authUser.IsAdmin { // Non-admin trying to schedule for someone else
			h.logger.WarnContext(ctx, "User not authorized to schedule message for another user", "auth_user_id", authUser.ID, "target_user_id", reqDTO.UserID)
			http.Error(w, "Not authorized to schedule messages for another user", http.StatusForbidden)
			return
		}
		targetUserID = reqDTO.UserID // Admin can specify, or user for themselves
	}

	if targetUserID == "" { // Should not happen if authUser.ID is always present
	    h.logger.ErrorContext(ctx, "Target UserID is empty after auth check for CreateScheduledMessage")
		http.Error(w, "Target UserID could not be determined", http.StatusInternalServerError)
		return
	}


	grpcReq := &pb.CreateScheduledMessageRequest{
		JobType:      reqDTO.JobType,
		Payload:      reqDTO.Payload,
		ScheduleTime: timestamppb.New(reqDTO.ScheduleTime),
		UserId:       targetUserID,
	}

	h.logger.InfoContext(ctx, "Sending CreateScheduledMessage request to gRPC service", "job_type", grpcReq.JobType, "user_id", grpcReq.UserId)
	grpcRes, err := h.schedulerClient.CreateScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatus(w, h.logger, err, "CreateScheduledMessage", "")
		return
	}

	resDTO := ScheduledMessageDTO{
		ID:           grpcRes.Id,
		JobType:      grpcRes.JobType,
		Payload:      grpcRes.Payload,
		ScheduleTime: grpcRes.ScheduleTime.AsTime(),
		CreatedAt:    grpcRes.CreatedAt.AsTime(),
		UpdatedAt:    grpcRes.UpdatedAt.AsTime(),
		Status:       grpcRes.Status,
		UserID:       grpcRes.UserId,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resDTO); err != nil {
		h.logger.ErrorContext(ctx, "Failed to encode response for CreateScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) GetScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	messageID := chi.URLParam(r, "id")
	if messageID == "" {
		h.logger.WarnContext(ctx, "Message ID is required for GetScheduledMessage")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	// Optional: Check if user is admin or owns the message
	authUser, _ := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)


	grpcReq := &pb.GetScheduledMessageRequest{Id: messageID}
	h.logger.InfoContext(ctx, "Sending GetScheduledMessage request to gRPC service", "id", messageID)
	grpcRes, err := h.schedulerClient.GetScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatus(w, h.logger, err, "GetScheduledMessage", messageID)
		return
	}

	if !authUser.IsAdmin && authUser.ID != grpcRes.UserId {
		h.logger.WarnContext(ctx, "User not authorized to access this scheduled message", "auth_user_id", authUser.ID, "message_owner_id", grpcRes.UserId)
		http.Error(w, "Not authorized to access this message", http.StatusForbidden)
		return
	}

	resDTO := ScheduledMessageDTO{
		ID:           grpcRes.Id,
		JobType:      grpcRes.JobType,
		Payload:      grpcRes.Payload,
		ScheduleTime: grpcRes.ScheduleTime.AsTime(),
		CreatedAt:    grpcRes.CreatedAt.AsTime(),
		UpdatedAt:    grpcRes.UpdatedAt.AsTime(),
		Status:       grpcRes.Status,
		UserID:       grpcRes.UserId,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resDTO); err != nil {
		h.logger.ErrorContext(ctx, "Failed to encode response for GetScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) ListScheduledMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queryParams := r.URL.Query()

	pageSize, _ := strconv.ParseInt(queryParams.Get("page_size"), 10, 32)
	pageNumber, _ := strconv.ParseInt(queryParams.Get("page_number"), 10, 32)
	statusFilter := queryParams.Get("status")
	jobTypeFilter := queryParams.Get("job_type")

	if pageSize <= 0 { pageSize = 10 }
	if pageNumber <= 0 { pageNumber = 1 }

	grpcReq := &pb.ListScheduledMessagesRequest{
		PageSize:   int32(pageSize),
		PageNumber: int32(pageNumber),
		Status:     statusFilter,
		JobType:    jobTypeFilter,
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok {
		h.logger.ErrorContext(ctx, "AuthenticatedUser not found in context for ListScheduledMessages")
		http.Error(w, "User authentication details not found", http.StatusUnauthorized)
		return
	}
	// If user is not admin, filter by their UserID
	if !authUser.IsAdmin {
		grpcReq.UserId = authUser.ID
	} else {
	    // Admin can optionally filter by a specific user_id from query params
	    if queryParams.Has("user_id") {
	        grpcReq.UserId = queryParams.Get("user_id")
        }
	}


	h.logger.InfoContext(ctx, "Sending ListScheduledMessages request to gRPC service", "params", grpcReq.String())
	grpcRes, err := h.schedulerClient.ListScheduledMessages(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatus(w, h.logger, err, "ListScheduledMessages", "")
		return
	}

	resDTOs := make([]ScheduledMessageDTO, len(grpcRes.Messages))
	for i, msg := range grpcRes.Messages {
		resDTOs[i] = ScheduledMessageDTO{
			ID:           msg.Id,
			JobType:      msg.JobType,
			Payload:      msg.Payload,
			ScheduleTime: msg.ScheduleTime.AsTime(),
			CreatedAt:    msg.CreatedAt.AsTime(),
			UpdatedAt:    msg.UpdatedAt.AsTime(),
			Status:       msg.Status,
			UserID:       msg.UserId,
		}
	}

	response := ListScheduledMessagesResponseDTO{
		Messages:   resDTOs,
		TotalCount: grpcRes.TotalCount,
		Page:       int32(pageNumber),
		PageSize:   int32(pageSize),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.ErrorContext(ctx, "Failed to encode response for ListScheduledMessages", "error", err)
	}
}

func (h *SchedulerHandler) UpdateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	messageID := chi.URLParam(r, "id")
	if messageID == "" {
		h.logger.WarnContext(ctx, "Message ID is required for UpdateScheduledMessage")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	var reqDTO UpdateScheduledMessageRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.logger.WarnContext(ctx, "Failed to decode request body for UpdateScheduledMessage", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		h.logger.WarnContext(ctx, "Validation failed for UpdateScheduledMessage", "error", err)
		http.Error(w, fmt.Sprintf("Validation error: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Check ownership or admin rights before proceeding
	authUser, _ := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !authUser.IsAdmin { // If not admin, fetch the message to check ownership
		peekReq := &pb.GetScheduledMessageRequest{Id: messageID}
		peekRes, err := h.schedulerClient.GetScheduledMessage(ctx, peekReq)
		if err != nil {
			mapGRPCErrorToHTTPStatus(w, h.logger, err, "UpdateScheduledMessage (peek)", messageID)
			return
		}
		if peekRes.UserId != authUser.ID {
			h.logger.WarnContext(ctx, "User not authorized to update this scheduled message", "auth_user_id", authUser.ID, "message_id", messageID)
			http.Error(w, "Not authorized to update this message", http.StatusForbidden)
			return
		}
	}
	// Admins can update any message; non-admins only their own.

	grpcReq := &pb.UpdateScheduledMessageRequest{
		Id:      messageID,
		Payload: reqDTO.Payload, // Will be nil if not provided due to omitempty on DTO and map
		Status:  reqDTO.Status,
	}
	if reqDTO.ScheduleTime != nil {
	    if reqDTO.ScheduleTime.Before(time.Now().Add(10*time.Second)) {
            h.logger.WarnContext(ctx, "Validation failed for UpdateScheduledMessage: ScheduleTime must be in the future.", "schedule_time", reqDTO.ScheduleTime)
            http.Error(w, "ScheduleTime must be at least 10 seconds in the future.", http.StatusBadRequest)
            return
        }
		grpcReq.ScheduleTime = timestamppb.New(*reqDTO.ScheduleTime)
	}

	h.logger.InfoContext(ctx, "Sending UpdateScheduledMessage request to gRPC service", "id", messageID, "payload_is_nil", grpcReq.Payload == nil, "scheduletime_is_nil", grpcReq.ScheduleTime == nil, "status", grpcReq.Status)
	grpcRes, err := h.schedulerClient.UpdateScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatus(w, h.logger, err, "UpdateScheduledMessage", messageID)
		return
	}

	resDTO := ScheduledMessageDTO{
		ID:           grpcRes.Id,
		JobType:      grpcRes.JobType,
		Payload:      grpcRes.Payload,
		ScheduleTime: grpcRes.ScheduleTime.AsTime(),
		CreatedAt:    grpcRes.CreatedAt.AsTime(),
		UpdatedAt:    grpcRes.UpdatedAt.AsTime(),
		Status:       grpcRes.Status,
		UserID:       grpcRes.UserId,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resDTO); err != nil {
		h.logger.ErrorContext(ctx, "Failed to encode response for UpdateScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) DeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	messageID := chi.URLParam(r, "id")
	if messageID == "" {
		h.logger.WarnContext(ctx, "Message ID is required for DeleteScheduledMessage")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	// Check ownership or admin rights before proceeding
	authUser, _ := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !authUser.IsAdmin { // If not admin, fetch the message to check ownership
		peekReq := &pb.GetScheduledMessageRequest{Id: messageID}
		peekRes, err := h.schedulerClient.GetScheduledMessage(ctx, peekReq)
		if err != nil {
			mapGRPCErrorToHTTPStatus(w, h.logger, err, "DeleteScheduledMessage (peek)", messageID)
			return
		}
		if peekRes.UserId != authUser.ID {
			h.logger.WarnContext(ctx, "User not authorized to delete this scheduled message", "auth_user_id", authUser.ID, "message_id", messageID)
			http.Error(w, "Not authorized to delete this message", http.StatusForbidden)
			return
		}
	}
	// Admins can delete any message; non-admins only their own if status allows (e.g. not 'processing')

	grpcReq := &pb.DeleteScheduledMessageRequest{Id: messageID}
	h.logger.InfoContext(ctx, "Sending DeleteScheduledMessage request to gRPC service", "id", messageID)
	_, err := h.schedulerClient.DeleteScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatus(w, h.logger, err, "DeleteScheduledMessage", messageID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers scheduler specific routes to a Chi router.
func (h *SchedulerHandler) RegisterRoutes(r chi.Router) {
	r.Post("/", h.CreateScheduledMessage)
	r.Get("/", h.ListScheduledMessages)
	r.Get("/{id}", h.GetScheduledMessage)
	r.Put("/{id}", h.UpdateScheduledMessage)
	r.Delete("/{id}", h.DeleteScheduledMessage)
}
