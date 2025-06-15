package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware" // For GetReqID
	"github.com/go-playground/validator/v10"
	"github.com/AradIT/aradsms/golang_services/internal/platform/messagebroker" // Corrected path
)

type IncomingHandler struct {
	natsClient messagebroker.NATSClient // Corrected type
	logger     *slog.Logger
	validate   *validator.Validate
}

// NewIncomingHandler creates a new IncomingHandler.
func NewIncomingHandler(nc messagebroker.NATSClient, logger *slog.Logger, validate *validator.Validate) *IncomingHandler { // Corrected type for nc
	return &IncomingHandler{
		natsClient: nc,
		logger:     logger.With("handler", "incoming"), // Add handler context to base logger
		validate:   validate,
	}
}

// HandleDLRCallback handles incoming DLR (Delivery Report) callbacks from SMS providers.
func (h *IncomingHandler) HandleDLRCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	providerName := chi.URLParam(r, "provider_name")
	if providerName == "" {
		logger.WarnContext(ctx, "Provider name missing in DLR callback URL")
		http.Error(w, "Provider name is required", http.StatusBadRequest)
		return
	}
	logger = logger.With("provider_name", providerName) // Add provider to logger context

	logger.InfoContext(ctx, "Received DLR callback")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read request body for DLR", "error", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var req ProviderDLRCallbackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.ErrorContext(ctx, "Failed to decode DLR request JSON", "error", err, "body", string(body))
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		logger.ErrorContext(ctx, "Failed to validate DLR request", "error", err, "request_data", req)
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	logger.InfoContext(ctx, "DLR request validated successfully", "message_id", req.MessageID, "status", req.Status)

	natsSubject := fmt.Sprintf("dlr.raw.%s", providerName)
	dataToPublish, err := json.Marshal(req)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal validated DLR request for NATS", "error", err)
		http.Error(w, "Internal server error preparing data for queue", http.StatusInternalServerError)
		return
	}

	if err := h.natsClient.Publish(ctx, natsSubject, dataToPublish); err != nil {
		logger.ErrorContext(ctx, "Failed to publish DLR to NATS", "error", err, "subject", natsSubject)
		http.Error(w, "Failed to queue DLR for processing", http.StatusInternalServerError)
		return
	}

	logger.InfoContext(ctx, "DLR successfully published to NATS", "subject", natsSubject, "message_id", req.MessageID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "DLR received and queued for processing"})
}

// HandleIncomingSMSCallback handles incoming SMS messages from SMS providers.
func (h *IncomingHandler) HandleIncomingSMSCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	providerName := chi.URLParam(r, "provider_name")
	if providerName == "" {
		logger.WarnContext(ctx, "Provider name missing in incoming SMS callback URL")
		http.Error(w, "Provider name is required", http.StatusBadRequest)
		return
	}
	logger = logger.With("provider_name", providerName)

	logger.InfoContext(ctx, "Received incoming SMS callback")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read request body for incoming SMS", "error", err)
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	var req ProviderIncomingSMSRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.ErrorContext(ctx, "Failed to decode incoming SMS request JSON", "error", err, "body", string(body))
		http.Error(w, "Invalid JSON format: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		logger.ErrorContext(ctx, "Failed to validate incoming SMS request", "error", err, "request_data", req)
		http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	logger.InfoContext(ctx, "Incoming SMS request validated successfully", "from", req.From, "to", req.To, "message_id", req.MessageID)

	natsSubject := fmt.Sprintf("sms.incoming.raw.%s", providerName)
	dataToPublish, err := json.Marshal(req)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal validated incoming SMS request for NATS", "error", err)
		http.Error(w, "Internal server error preparing data for queue", http.StatusInternalServerError)
		return
	}

	if err := h.natsClient.Publish(ctx, natsSubject, dataToPublish); err != nil {
		logger.ErrorContext(ctx, "Failed to publish incoming SMS to NATS", "error", err, "subject", natsSubject)
		http.Error(w, "Failed to queue incoming SMS for processing", http.StatusInternalServerError)
		return
	}

	logger.InfoContext(ctx, "Incoming SMS successfully published to NATS", "subject", natsSubject, "message_id", req.MessageID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "Incoming SMS received and queued for processing"})
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
