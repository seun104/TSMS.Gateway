package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/aradsms/golang_services/internal/core_sms/domain"
	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	outboxRepoIface "github.com/aradsms/golang_services/internal/sms_sending_service/repository"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware" // For GetReqID
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SendMessageRequest DTO for POST /messages/send
type SendMessageRequest struct {
	SenderID  string  `json:"sender_id" validate:"required"`
	Recipient string  `json:"recipient" validate:"required"` // TODO: Add phone number validation
	Content   string  `json:"content" validate:"required,min=1"`
	UserData  *string `json:"user_data,omitempty"`
}

// SendMessageResponse DTO
type SendMessageResponse struct {
	MessageID string              `json:"message_id"`
	Status    domain.MessageStatus `json:"status"`
	Recipient string              `json:"recipient"`
}

// MessageStatusResponse DTO for GET /messages/{message_id}
type MessageStatusResponse struct {
	ID                  string               `json:"id"`
	UserID              string               `json:"user_id"`
	SenderID            string               `json:"sender_id"`
	Recipient           string               `json:"recipient"`
	Content             string               `json:"content,omitempty"` // Optional to return full content
	Status              domain.MessageStatus `json:"status"`
	Segments            int                  `json:"segments"`
	ProviderMessageID   *string              `json:"provider_message_id,omitempty"`
	ProviderStatusCode  *string              `json:"provider_status_code,omitempty"`
	ErrorMessage        *string              `json:"error_message,omitempty"`
	UserData            *string              `json:"user_data,omitempty"`
	ScheduledFor        *time.Time           `json:"scheduled_for,omitempty"`
	ProcessedAt         *time.Time           `json:"processed_at,omitempty"`
	SentToProviderAt    *time.Time           `json:"sent_to_provider_at,omitempty"`
	DeliveredAt         *time.Time           `json:"delivered_at,omitempty"`
	LastStatusUpdateAt  *time.Time           `json:"last_status_update_at,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}


type MessageHandler struct {
	natsClient *messagebroker.NatsClient
	outboxRepo outboxRepoIface.OutboxRepository
	dbPool     *pgxpool.Pool
	logger     *slog.Logger
}

func NewMessageHandler(
	natsClient *messagebroker.NatsClient,
	outboxRepo outboxRepoIface.OutboxRepository,
	dbPool *pgxpool.Pool,
	logger *slog.Logger,
) *MessageHandler {
	return &MessageHandler{
		natsClient: natsClient,
		outboxRepo: outboxRepo,
		dbPool:     dbPool,
		logger:     logger.With("handler", "message"),
	}
}

// RegisterRoutes registers message routes with the given router.
func (h *MessageHandler) RegisterRoutes(r chi.Router) {
	r.Post("/messages/send", h.handleSendMessage)
	r.Get("/messages/{messageID}", h.handleGetMessageStatus) // New route
}

// handleSendMessage (already implemented)
func (h *MessageHandler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok || authUser.ID == "" {
		logger.WarnContext(ctx, "User not authenticated for send message")
		h.jsonError(w, logger, "User not authenticated", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authUser.ID) // Add UserID to logger context

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.ErrorContext(ctx, "Failed to decode send message request", "error", err)
		h.jsonError(w, logger, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: Add validation for req struct using h.validate

	outboxMessageID := uuid.NewString()
	now := time.Now()
	initialMessage := &domain.OutboxMessage{
		ID: outboxMessageID, UserID: authUser.ID, SenderID: req.SenderID, Recipient: req.Recipient,
		Content: req.Content, Status: domain.MessageStatusQueued, Segments: 1, UserData: req.UserData, // TODO: Calculate segments
		CreatedAt: now, UpdatedAt: now,
	}
	logger.InfoContext(ctx, "Attempting to create outbox message", "message_id", outboxMessageID)
	_, err := h.outboxRepo.Create(ctx, h.dbPool, initialMessage) // Assuming Create takes ctx
	if err != nil {
		logger.ErrorContext(ctx, "Failed to create initial outbox message record", "error", err, "message_id", outboxMessageID)
		h.jsonError(w, logger, "Failed to queue message (database error)", http.StatusInternalServerError)
		return
	}
	natsPayload := map[string]string{"outbox_message_id": outboxMessageID}
	payloadBytes, marshalErr := json.Marshal(natsPayload)
	if marshalErr != nil {
		logger.ErrorContext(ctx, "Failed to marshal NATS payload for send message", "error", marshalErr, "message_id", outboxMessageID)
		h.jsonError(w, logger, "Failed to prepare message for sending queue", http.StatusInternalServerError)
		return
	}
	subject := "sms.jobs.send"
	if pubErr := h.natsClient.Publish(ctx, subject, payloadBytes); pubErr != nil { // Pass ctx to Publish
		logger.ErrorContext(ctx, "Failed to publish send SMS job to NATS", "error", pubErr, "message_id", outboxMessageID, "subject", subject)
		h.jsonError(w, logger, "Failed to send message to processing queue", http.StatusInternalServerError)
		return
	}
	logger.InfoContext(ctx, "Send SMS job published to NATS", "message_id", outboxMessageID, "subject", subject)
	response := SendMessageResponse{
		MessageID: outboxMessageID, Status: domain.MessageStatusQueued, Recipient: req.Recipient,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}


// handleGetMessageStatus retrieves the status of a specific message.
func (h *MessageHandler) handleGetMessageStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	authUser, ok := ctx.Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok || authUser.ID == "" {
		logger.WarnContext(ctx, "User not authenticated for get message status")
		h.jsonError(w, logger, "User not authenticated", http.StatusUnauthorized)
		return
	}
	logger = logger.With("auth_user_id", authUser.ID)

	messageID := chi.URLParam(r, "messageID")
	if _, err := uuid.Parse(messageID); err != nil {
		logger.WarnContext(ctx, "Invalid message ID format provided", "message_id", messageID, "error", err)
		h.jsonError(w, logger, "Invalid message ID format", http.StatusBadRequest)
		return
	}

	logger.InfoContext(ctx, "Fetching message status", "message_id", messageID)

	outboxMsg, err := h.outboxRepo.GetByID(ctx, h.dbPool, messageID) // Pass ctx
	if err != nil {
		if errors.Is(err, outboxRepoIface.ErrOutboxMessageNotFound) {
			logger.InfoContext(ctx, "Outbox message not found by ID", "message_id", messageID)
			h.jsonError(w, logger, "Message not found", http.StatusNotFound)
			return
		}
		logger.ErrorContext(ctx, "Failed to get outbox message from repository", "error", err, "message_id", messageID)
		h.jsonError(w, logger, "Failed to retrieve message status", http.StatusInternalServerError)
		return
	}

	if outboxMsg.UserID != authUser.ID && !authUser.IsAdmin {
		logger.WarnContext(ctx, "User attempted to access unauthorized message", "message_id", messageID, "message_owner_id", outboxMsg.UserID)
		h.jsonError(w, logger, "Forbidden: You do not have permission to view this message", http.StatusForbidden)
		return
	}
	logger.DebugContext(ctx, "Message status retrieved successfully", "message_id", messageID, "status", outboxMsg.Status)

	response := MessageStatusResponse{
		ID:                  outboxMsg.ID,
		UserID:              outboxMsg.UserID,
		SenderID:            outboxMsg.SenderID,
		Recipient:           outboxMsg.Recipient,
		// Content:          outboxMsg.Content, // Decide if content should be returned here
		Status:              outboxMsg.Status,
		Segments:            outboxMsg.Segments,
		ProviderMessageID:   outboxMsg.ProviderMessageID,
		ProviderStatusCode:  outboxMsg.ProviderStatusCode,
		ErrorMessage:        outboxMsg.ErrorMessage,
		UserData:            outboxMsg.UserData,
		ScheduledFor:        outboxMsg.ScheduledFor,
		ProcessedAt:         outboxMsg.ProcessedAt,
		SentToProviderAt:    outboxMsg.SentToProviderAt,
		DeliveredAt:         outboxMsg.DeliveredAt,
		LastStatusUpdateAt:  outboxMsg.LastStatusUpdateAt,
		CreatedAt:           outboxMsg.CreatedAt,
		UpdatedAt:           outboxMsg.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// jsonError helper (already defined or shared) - needs to accept logger
func (h *MessageHandler) jsonError(w http.ResponseWriter, logger *slog.Logger, message string, statusCode int) {
	logger.WarnContext(context.Background(), "API Error Response", "status_code", statusCode, "message", message) // Use a background context if request context might be cancelled
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(GenericErrorResponse{Error: message}) // GenericErrorResponse needs to be defined or use map
}

// GenericErrorResponse can be defined in a shared DTOs file or locally if not already.
type GenericErrorResponse struct {
	Error string `json:"error"`
}
