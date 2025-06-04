package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10" // Assuming validator is used
	"github.com/your-repo/project/internal/platform/messagebroker"
	// Assuming a common response utility, or define simple ones here
	// "github.com/your-repo/project/internal/public_api_service/transport/http/response"
)

type IncomingHandler struct {
	natsClient *messagebroker.NATSClient // Using the existing NATSClient wrapper
	logger     *slog.Logger
	validate   *validator.Validate // Standard validator
}

// NewIncomingHandler creates a new IncomingHandler.
func NewIncomingHandler(nc *messagebroker.NATSClient, logger *slog.Logger, validate *validator.Validate) *IncomingHandler {
	return &IncomingHandler{
		natsClient: nc,
		logger:     logger,
		validate:   validate,
	}
}

// HandleDLRCallback handles incoming DLR (Delivery Report) callbacks from SMS providers.
func (h *IncomingHandler) HandleDLRCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerName := chi.URLParam(r, "provider_name")
	if providerName == "" {
		h.logger.WarnContext(ctx, "Provider name missing in DLR callback URL")
		http.Error(w, "Provider name is required", http.StatusBadRequest) // Using http.Error for simplicity
		return
	}

	h.logger.InfoContext(ctx, "Received DLR callback", "provider", providerName)

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to read request body", "error", err, "provider", providerName)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Decode the JSON request body into ProviderDLRCallbackRequest
	var req ProviderDLRCallbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to decode DLR request JSON", "error", err, "provider", providerName, "body", string(body))
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the DTO
	if err := h.validate.StructCtx(ctx, req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to validate DLR request", "error", err, "provider", providerName, "request_data", req)
		// TODO: Provide more detailed validation errors to the client if necessary
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.InfoContext(ctx, "DLR request validated successfully", "provider", providerName, "message_id", req.MessageID, "status", req.Status)

	// Construct NATS subject: dlr.raw.{provider_name}
	natsSubject := fmt.Sprintf("dlr.raw.%s", providerName)

	// Data to publish can be the raw body (if trusted and validated indirectly)
	// or the marshaled validated struct (req). Marshaling 'req' ensures we publish what we validated.
	dataToPublish, err := json.Marshal(req)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to marshal validated DLR request for NATS", "error", err, "provider", providerName)
		http.Error(w, "Internal server error preparing data for queue", http.StatusInternalServerError)
		return
	}

	// Publish to NATS
	if err := h.natsClient.Publish(ctx, natsSubject, dataToPublish); err != nil {
		h.logger.ErrorContext(ctx, "Failed to publish DLR to NATS", "error", err, "subject", natsSubject, "provider", providerName)
		http.Error(w, "Failed to queue DLR for processing", http.StatusInternalServerError)
		return
	}

	h.logger.InfoContext(ctx, "DLR successfully published to NATS", "subject", natsSubject, "provider", providerName, "message_id", req.MessageID)

	// Respond with HTTP 202 Accepted, as processing is asynchronous.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "DLR received and queued for processing"})
}

// HandleIncomingSMSCallback handles incoming SMS messages from SMS providers.
func (h *IncomingHandler) HandleIncomingSMSCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerName := chi.URLParam(r, "provider_name")
	if providerName == "" {
		h.logger.WarnContext(ctx, "Provider name missing in incoming SMS callback URL")
		http.Error(w, "Provider name is required", http.StatusBadRequest)
		return
	}

	h.logger.InfoContext(ctx, "Received incoming SMS callback", "provider", providerName)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to read request body for incoming SMS", "error", err, "provider", providerName)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var req ProviderIncomingSMSRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to decode incoming SMS request JSON", "error", err, "provider", providerName, "body", string(body))
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to validate incoming SMS request", "error", err, "provider", providerName, "request_data", req)
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.InfoContext(ctx, "Incoming SMS request validated successfully",
		"provider", providerName,
		"from", req.From,
		"to", req.To,
		"message_id", req.MessageID,
	)

	natsSubject := fmt.Sprintf("sms.incoming.raw.%s", providerName)

	dataToPublish, err := json.Marshal(req)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to marshal validated incoming SMS request for NATS", "error", err, "provider", providerName)
		http.Error(w, "Internal server error preparing data for queue", http.StatusInternalServerError)
		return
	}

	if err := h.natsClient.Publish(ctx, natsSubject, dataToPublish); err != nil {
		h.logger.ErrorContext(ctx, "Failed to publish incoming SMS to NATS", "error", err, "subject", natsSubject, "provider", providerName)
		http.Error(w, "Failed to queue incoming SMS for processing", http.StatusInternalServerError)
		return
	}

	h.logger.InfoContext(ctx, "Incoming SMS successfully published to NATS", "subject", natsSubject, "provider", providerName, "message_id", req.MessageID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "Incoming SMS received and queued for processing"})
}

/*
Example of a simple JSON response utility (can be in a separate 'response' package)

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
*/
