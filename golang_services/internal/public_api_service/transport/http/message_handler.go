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
	outboxRepoIface "github.com/aradsms/golang_services/internal/sms_sending_service/repository" // Interface
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"     // For AuthenticatedUserContextKey
	"github.com/go-chi/chi/v5" // Import chi for URL param
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
	authUser, ok := r.Context().Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok || authUser.ID == "" {
		h.jsonError(w, "User not authenticated", http.StatusUnauthorized)
		return
	}
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	outboxMessageID := uuid.NewString()
	now := time.Now()
	initialMessage := &domain.OutboxMessage{
		ID: outboxMessageID, UserID: authUser.ID, SenderID: req.SenderID, Recipient: req.Recipient,
		Content: req.Content, Status: domain.MessageStatusQueued, Segments: 1, UserData: req.UserData,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := h.outboxRepo.Create(r.Context(), h.dbPool, initialMessage)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "Failed to create initial outbox message record", "error", err)
		h.jsonError(w, "Failed to queue message (database error)", http.StatusInternalServerError)
		return
	}
	natsPayload := map[string]string{"outbox_message_id": outboxMessageID}
	payloadBytes, marshalErr := json.Marshal(natsPayload)
	if marshalErr != nil {
		h.logger.ErrorContext(r.Context(), "Failed to marshal NATS payload", "error", marshalErr)
		h.jsonError(w, "Failed to prepare message for sending queue", http.StatusInternalServerError)
		return
	}
	subject := "sms.jobs.send"
	if err := h.natsClient.Publish(r.Context(), subject, payloadBytes); err != nil {
		h.logger.ErrorContext(r.Context(), "Failed to publish send SMS job to NATS", "error", err)
		h.jsonError(w, "Failed to send message to processing queue", http.StatusInternalServerError)
		return
	}
	response := SendMessageResponse{
		MessageID: outboxMessageID, Status: domain.MessageStatusQueued, Recipient: req.Recipient,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}


// handleGetMessageStatus retrieves the status of a specific message.
func (h *MessageHandler) handleGetMessageStatus(w http.ResponseWriter, r *http.Request) {
	authUser, ok := r.Context().Value(middleware.AuthenticatedUserContextKey).(middleware.AuthenticatedUser)
	if !ok || authUser.ID == "" {
		h.jsonError(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	messageID := chi.URLParam(r, "messageID")
	if _, err := uuid.Parse(messageID); err != nil { // Validate if messageID is a UUID
		h.jsonError(w, "Invalid message ID format", http.StatusBadRequest)
		return
	}

	h.logger.InfoContext(r.Context(), "Fetching message status", "message_id", messageID, "user_id", authUser.ID)

	// The dbPool itself can act as a Querier for non-transactional reads
	outboxMsg, err := h.outboxRepo.GetByID(r.Context(), h.dbPool, messageID)
	if err != nil {
		if errors.Is(err, outboxRepoIface.ErrOutboxMessageNotFound) { // Use interface defined error
			h.logger.WarnContext(r.Context(), "Outbox message not found", "message_id", messageID)
			h.jsonError(w, "Message not found", http.StatusNotFound)
			return
		}
		h.logger.ErrorContext(r.Context(), "Failed to get outbox message from repository", "error", err, "message_id", messageID)
		h.jsonError(w, "Failed to retrieve message status", http.StatusInternalServerError)
		return
	}

	// Security check: Ensure the authenticated user is the owner of the message
	if outboxMsg.UserID != authUser.ID && !authUser.IsAdmin { // Admins can see all messages
		h.logger.WarnContext(r.Context(), "User attempted to access unauthorized message", "message_id", messageID, "auth_user_id", authUser.ID, "message_owner_id", outboxMsg.UserID)
		h.jsonError(w, "Forbidden: You do not have permission to view this message", http.StatusForbidden)
		return
	}

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

// jsonError helper (already defined or shared)
func (h *MessageHandler) jsonError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(GenericErrorResponse{Error: message})
}
