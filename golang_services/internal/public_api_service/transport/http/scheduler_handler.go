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
	// "google.golang.org/protobuf/types/known/emptypb" // Not directly used in this diff, but likely needed if Delete returns specific type
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	chi_middleware "github.com/go-chi/chi/v5/middleware" // For GetReqID

	pb "github.com/AradIT/Arad.SMS.Gateway/golang_services/api/proto/schedulerservice"
)

type SchedulerHandler struct {
	schedulerClient pb.SchedulerServiceClient
	logger          *slog.Logger // Base logger
	validate        *validator.Validate
}

func NewSchedulerHandler(client pb.SchedulerServiceClient, logger *slog.Logger, validate *validator.Validate) *SchedulerHandler {
	return &SchedulerHandler{
		schedulerClient: client,
		logger:          logger.With("handler", "scheduler"), // Contextualize base logger
		validate:        validate,
	}
}

// Helper to map gRPC errors to HTTP status codes - now uses the request-scoped logger
func mapGRPCErrorToHTTPStatusAndRespond(w http.ResponseWriter, logger *slog.Logger, err error, operation string, resourceID string) {
	if err == nil {
		return
	}
	st, ok := status.FromError(err)
	if !ok {
		// Use the provided logger which should be request-scoped
		logger.ErrorContext(context.Background(), // Consider passing original request context if available and safe
			fmt.Sprintf("%s failed: unable to parse gRPC error", operation),
			"resource_id", resourceID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// logger here is already request-scoped
	logEntry := logger.With("grpc_code", st.Code().String(), "grpc_message", st.Message(), "operation", operation)
	if resourceID != "" {
		logEntry = logEntry.With("resource_id", resourceID)
	}


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
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	var reqDTO CreateScheduledMessageRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		logger.WarnContext(ctx, "Failed to decode request body for CreateScheduledMessage", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		logger.WarnContext(ctx, "Validation failed for CreateScheduledMessage", "error", err, "dto", reqDTO)
		http.Error(w, fmt.Sprintf("Validation error: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if reqDTO.ScheduleTime.Before(time.Now().Add(10*time.Second)) {
		logger.WarnContext(ctx, "Validation failed: ScheduleTime must be in the future.", "schedule_time", reqDTO.ScheduleTime)
		http.Error(w, "ScheduleTime must be at least 10 seconds in the future.", http.StatusBadRequest)
		return
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser) // Corrected to pointer
	if !ok || authUser == nil {
		logger.ErrorContext(ctx, "AuthenticatedUser not found in context")
		http.Error(w, "User authentication details not found", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authUser.ID)


	targetUserID := authUser.ID
	if reqDTO.UserID != "" {
		if reqDTO.UserID != authUser.ID && !authUser.IsAdmin {
			logger.WarnContext(ctx, "User not authorized to schedule message for another user", "target_user_id", reqDTO.UserID)
			http.Error(w, "Not authorized to schedule messages for another user", http.StatusForbidden)
			return
		}
		targetUserID = reqDTO.UserID
	}

	if targetUserID == "" {
	    logger.ErrorContext(ctx, "Target UserID is empty after auth check")
		http.Error(w, "Target UserID could not be determined", http.StatusInternalServerError)
		return
	}
	logger = logger.With("target_user_id_for_message", targetUserID)


	grpcReq := &pb.CreateScheduledMessageRequest{
		JobType:      reqDTO.JobType,
		Payload:      reqDTO.Payload,
		ScheduleTime: timestamppb.New(reqDTO.ScheduleTime),
		UserId:       targetUserID,
	}

	logger.InfoContext(ctx, "Sending CreateScheduledMessage request to gRPC service", "job_type", grpcReq.JobType)
	grpcRes, err := h.schedulerClient.CreateScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "CreateScheduledMessage", "") // Use updated helper
		return
	}
	logger.InfoContext(ctx, "Scheduled message created successfully", "message_id", grpcRes.Id)

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
		logger.ErrorContext(ctx, "Failed to encode response for CreateScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) GetScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	messageID := chi.URLParam(r, "id")
	logger = logger.With("message_id", messageID)

	if messageID == "" { // Should be caught by Chi route, but good practice
		logger.WarnContext(ctx, "Message ID is required")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser)
	if !ok || authUser == nil {
		logger.ErrorContext(ctx, "AuthenticatedUser not found in context")
		http.Error(w, "User authentication details not found", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authUser.ID)
	logger.InfoContext(ctx, "Attempting to get scheduled message")


	grpcReq := &pb.GetScheduledMessageRequest{Id: messageID}
	grpcRes, err := h.schedulerClient.GetScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "GetScheduledMessage", messageID) // Use updated helper
		return
	}

	if !authUser.IsAdmin && authUser.ID != grpcRes.UserId {
		logger.WarnContext(ctx, "User not authorized to access this scheduled message", "message_owner_id", grpcRes.UserId)
		http.Error(w, "Not authorized to access this message", http.StatusForbidden)
		return
	}
	logger.InfoContext(ctx, "Scheduled message retrieved successfully")

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
		logger.ErrorContext(ctx, "Failed to encode response for GetScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) ListScheduledMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser)
	if !ok || authUser == nil {
		logger.ErrorContext(ctx, "AuthenticatedUser not found in context")
		http.Error(w, "User authentication details not found", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authUser.ID)
	logger.InfoContext(ctx, "Attempting to list scheduled messages")

	queryParams := r.URL.Query()
	pageSize, _ := strconv.ParseInt(queryParams.Get("page_size"), 10, 32)
	pageNumber, _ := strconv.ParseInt(queryParams.Get("page_number"), 10, 32)
	statusFilter := queryParams.Get("status")
	jobTypeFilter := queryParams.Get("job_type")
	userIDFilter := queryParams.Get("user_id") // For admin to filter by specific user

	if pageSize <= 0 { pageSize = 10 }
	if pageNumber <= 0 { pageNumber = 1 }

	grpcReq := &pb.ListScheduledMessagesRequest{
		PageSize:   int32(pageSize),
		PageNumber: int32(pageNumber),
		Status:     statusFilter,
		JobType:    jobTypeFilter,
	}

	if !authUser.IsAdmin {
		grpcReq.UserId = authUser.ID // Non-admins can only list their own
	} else if userIDFilter != "" {
		grpcReq.UserId = userIDFilter // Admin specified a user to filter by
	}
	// If admin and userIDFilter is empty, gRPC service should list for all users (or handle as needed)

	logger.DebugContext(ctx, "Sending ListScheduledMessages request to gRPC service", "params", grpcReq.String())
	grpcRes, err := h.schedulerClient.ListScheduledMessages(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "ListScheduledMessages", "") // Use updated helper
		return
	}
	logger.InfoContext(ctx, "Scheduled messages listed successfully", "count", len(grpcRes.Messages), "total_count", grpcRes.TotalCount)

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
		logger.ErrorContext(ctx, "Failed to encode response for ListScheduledMessages", "error", err)
	}
}

func (h *SchedulerHandler) UpdateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	messageID := chi.URLParam(r, "id")
	logger = logger.With("message_id", messageID)

	if messageID == "" {
		logger.WarnContext(ctx, "Message ID is required")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	var reqDTO UpdateScheduledMessageRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		logger.WarnContext(ctx, "Failed to decode UpdateScheduledMessage request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close() // Ensure body is closed

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		logger.WarnContext(ctx, "Validation failed for UpdateScheduledMessage", "error", err, "dto", reqDTO)
		http.Error(w, fmt.Sprintf("Validation error: %s", err.Error()), http.StatusBadRequest)
		return
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser)
    if !ok || authUser == nil {
        logger.ErrorContext(ctx, "AuthenticatedUser not found in context")
        http.Error(w, "User authentication details not found", http.StatusUnauthorized)
        return
    }
    logger = logger.With("auth_user_id", authUser.ID)
	logger.InfoContext(ctx, "Attempting to update scheduled message")


	if !authUser.IsAdmin {
		peekReq := &pb.GetScheduledMessageRequest{Id: messageID}
		peekRes, err := h.schedulerClient.GetScheduledMessage(ctx, peekReq)
		if err != nil {
			mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "UpdateScheduledMessage (owner check)", messageID)
			return
		}
		if peekRes.UserId != authUser.ID {
			logger.WarnContext(ctx, "User not authorized to update this scheduled message", "message_owner_id", peekRes.UserId)
			http.Error(w, "Not authorized to update this message", http.StatusForbidden)
			return
		}
	}

	grpcReq := &pb.UpdateScheduledMessageRequest{
		Id:      messageID,
		Payload: reqDTO.Payload,
		Status:  reqDTO.Status,
	}
	if reqDTO.ScheduleTime != nil {
	    if reqDTO.ScheduleTime.Before(time.Now().Add(10*time.Second)) { // Check only if time is actually being updated
            logger.WarnContext(ctx, "Validation failed: ScheduleTime must be in the future for update.", "schedule_time", reqDTO.ScheduleTime)
            http.Error(w, "ScheduleTime must be at least 10 seconds in the future.", http.StatusBadRequest)
            return
        }
		grpcReq.ScheduleTime = timestamppb.New(*reqDTO.ScheduleTime)
	}

	logger.InfoContext(ctx, "Sending UpdateScheduledMessage request to gRPC service", "payload_is_nil", grpcReq.Payload == nil, "scheduletime_is_nil", grpcReq.ScheduleTime == nil, "status", grpcReq.Status)
	grpcRes, err := h.schedulerClient.UpdateScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "UpdateScheduledMessage", messageID)
		return
	}
	logger.InfoContext(ctx, "Scheduled message updated successfully")

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
		logger.ErrorContext(ctx, "Failed to encode response for UpdateScheduledMessage", "error", err)
	}
}

func (h *SchedulerHandler) DeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	messageID := chi.URLParam(r, "id")
	logger = logger.With("message_id", messageID)

	if messageID == "" {
		logger.WarnContext(ctx, "Message ID is required")
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser)
    if !ok || authUser == nil {
        logger.ErrorContext(ctx, "AuthenticatedUser not found in context")
        http.Error(w, "User authentication details not found", http.StatusUnauthorized)
        return
    }
    logger = logger.With("auth_user_id", authUser.ID)
	logger.InfoContext(ctx, "Attempting to delete scheduled message")


	if !authUser.IsAdmin {
		peekReq := &pb.GetScheduledMessageRequest{Id: messageID}
		peekRes, err := h.schedulerClient.GetScheduledMessage(ctx, peekReq)
		if err != nil {
			mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "DeleteScheduledMessage (owner check)", messageID)
			return
		}
		if peekRes.UserId != authUser.ID {
			logger.WarnContext(ctx, "User not authorized to delete this scheduled message", "message_owner_id", peekRes.UserId)
			http.Error(w, "Not authorized to delete this message", http.StatusForbidden)
			return
		}
	}

	grpcReq := &pb.DeleteScheduledMessageRequest{Id: messageID}
	logger.DebugContext(ctx, "Sending DeleteScheduledMessage request to gRPC service")
	_, err := h.schedulerClient.DeleteScheduledMessage(ctx, grpcReq)
	if err != nil {
		mapGRPCErrorToHTTPStatusAndRespond(w, logger, err, "DeleteScheduledMessage", messageID)
		return
	}
	logger.InfoContext(ctx, "Scheduled message deleted successfully")

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
