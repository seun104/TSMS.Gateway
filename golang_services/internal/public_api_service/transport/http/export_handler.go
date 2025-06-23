package http

import (
	"encoding/json"
	"net/http"
	"log/slog"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	chi_middleware "github.com/go-chi/chi/v5/middleware" // For GetReqID

	exportDomain "github.com/AradIT/aradsms/golang_services/internal/export_service/domain"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/middleware"
)

type ExportHandler struct {
	natsClient messagebroker.NATSClient
	logger     *slog.Logger
	validate   *validator.Validate
}

func NewExportHandler(natsClient messagebroker.NATSClient, logger *slog.Logger, validate *validator.Validate) *ExportHandler {
	return &ExportHandler{
		natsClient: natsClient,
		logger:     logger.With("component", "export_handler"),
		validate:   validate,
	}
}

func (h *ExportHandler) RequestExportOutboxMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	authDetails, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser)
	if !ok || authDetails == nil {
		logger.WarnContext(ctx, "Unauthorized export request: Missing authentication details")
		http.Error(w, "Unauthorized: Missing or invalid authentication details", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authDetails.UserID) // Add UserID to logger context

	var reqDTO CreateExportRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		logger.ErrorContext(ctx, "Failed to decode request body for export request", "error", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil {
		logger.ErrorContext(ctx, "Validation failed for export request", "error", err, "dto", reqDTO)
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if reqDTO.EntityType != "outbox_messages" {
		 logger.WarnContext(ctx, "Invalid entity_type for export", "entity_type", reqDTO.EntityType)
		 http.Error(w, "Invalid entity_type for export. Only 'outbox_messages' is supported.", http.StatusBadRequest)
		 return
	}

	userID, err := uuid.Parse(authDetails.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to parse UserID from auth context", "error", err) // auth_user_id already in logger
		http.Error(w, "Internal server error: Invalid user identity", http.StatusInternalServerError)
		return
	}

	requesterEmail := ""
	if authDetails.Email != "" {
		requesterEmail = authDetails.Email
	} else {
		logger.WarnContext(ctx, "Requester email not found in auth details for export notification")
	}


	event := exportDomain.ExportRequestEvent{
		UserID:         userID,
		Filters:        reqDTO.Filters,
		RequesterEmail: requesterEmail,
	}
	payloadBytes, err := json.Marshal(event)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal export request event", "error", err)
		http.Error(w, "Internal server error publishing export request", http.StatusInternalServerError)
		return
	}

	if err := h.natsClient.Publish(ctx, exportDomain.NATSExportRequestOutboxV1, payloadBytes); err != nil {
		logger.ErrorContext(ctx, "Failed to publish export request to NATS", "error", err)
		http.Error(w, "Failed to request export due to internal error", http.StatusInternalServerError)
		return
	}

	logger.InfoContext(ctx, "Export request published to NATS", "filters", reqDTO.Filters, "email", requesterEmail)

	response := CreateExportResponseDTO{
		Message: "Export request accepted and is being processed.",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.ErrorContext(ctx, "Failed to write success response for export request", "error", err)
	}
}
