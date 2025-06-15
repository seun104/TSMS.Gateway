package http

import (
	"encoding/json"
	"net/http"
	"log/slog"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

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
	authDetails, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(*middleware.AuthenticatedUser) // Corrected to pointer type
	if !ok || authDetails == nil {
		h.logger.WarnContext(ctx, "Unauthorized export request: Missing authentication details")
		http.Error(w, "Unauthorized: Missing or invalid authentication details", http.StatusUnauthorized)
		return
	}

	var reqDTO CreateExportRequestDTO // This DTO is defined in export_dtos.go in the same package
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		h.logger.ErrorContext(ctx, "Failed to decode request body for export request", "error", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := h.validate.StructCtx(ctx, reqDTO); err != nil { // Use StructCtx for context-aware validation
		h.logger.ErrorContext(ctx, "Validation failed for export request", "error", err, "dto", reqDTO)
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// EntityType is validated by tag, but double check for safety or future expansion
	if reqDTO.EntityType != "outbox_messages" {
		 h.logger.WarnContext(ctx, "Invalid entity_type for export", "entity_type", reqDTO.EntityType)
		 http.Error(w, "Invalid entity_type for export. Only 'outbox_messages' is supported.", http.StatusBadRequest)
		 return
	}

	userID, err := uuid.Parse(authDetails.UserID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to parse UserID from auth context", "auth_user_id", authDetails.UserID, "error", err)
		http.Error(w, "Internal server error: Invalid user identity", http.StatusInternalServerError)
		return
	}

	requesterEmail := "" // Assuming AuthenticatedUser struct has Email field
	if authDetails.Email != "" {
		requesterEmail = authDetails.Email
	} else {
		h.logger.WarnContext(ctx, "Requester email not found in auth details for export notification", "user_id", userID)
	}


	event := exportDomain.ExportRequestEvent{
		UserID:         userID,
		Filters:        reqDTO.Filters,
		RequesterEmail: requesterEmail,
	}
	payloadBytes, err := json.Marshal(event)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to marshal export request event", "error", err, "user_id", userID)
		http.Error(w, "Internal server error publishing export request", http.StatusInternalServerError)
		return
	}

	if err := h.natsClient.Publish(ctx, exportDomain.NATSExportRequestOutboxV1, payloadBytes); err != nil {
		h.logger.ErrorContext(ctx, "Failed to publish export request to NATS", "error", err, "user_id", userID)
		http.Error(w, "Failed to request export due to internal error", http.StatusInternalServerError)
		return
	}

	h.logger.InfoContext(ctx, "Export request published to NATS", "user_id", userID, "filters", reqDTO.Filters, "email", requesterEmail)

	response := CreateExportResponseDTO{ // This DTO is defined in export_dtos.go in the same package
		Message: "Export request accepted and is being processed.",
		// RequestID: // Could generate and return a tracking ID here if needed
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write success response for export request", "error", err)
	}
}
