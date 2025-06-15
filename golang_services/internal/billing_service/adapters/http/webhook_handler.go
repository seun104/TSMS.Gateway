package http

import (
	"context"
	"io"
	"net/http"
	"log/slog"
	"strings" // For error message checking

	// Corrected import for billing app service (assuming BillingAppService struct is in 'app' package)
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/app"
	chi_middleware "github.com/go-chi/chi/v5/middleware" // For GetReqID
)

const MaxRequestBodySize = 1 << 20 // 1 MB

// PaymentWebhookProcessor defines the interface required by the WebhookHandler
// for processing payment events. This makes testing easier by allowing mocks.
type PaymentWebhookProcessor interface {
	HandlePaymentWebhook(ctx context.Context, rawPayload []byte, signature string) error
}

type WebhookHandler struct {
	appService PaymentWebhookProcessor // Use the interface type
	logger     *slog.Logger
}

func NewWebhookHandler(appService PaymentWebhookProcessor, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		appService: appService,
		logger:     logger.With("component", "webhook_handler"),
	}
}

// HandlePaymentWebhook receives webhook events from a payment gateway.
func (h *WebhookHandler) HandlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	if r.Method != http.MethodPost {
		logger.WarnContext(ctx, "Method not allowed for webhook", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	signature := r.Header.Get("X-Payment-Signature")
	logger = logger.With("signature_present", signature != "")


	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	rawPayload, err := io.ReadAll(r.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read webhook request body", "error", err)
		if err.Error() == "http: request body too large" {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
		}
		return
	}

	logger.InfoContext(ctx, "Received payment webhook",
		"remote_addr", r.RemoteAddr,
		"content_length", r.ContentLength,
		"payload_size", len(rawPayload))

	if err := h.appService.HandlePaymentWebhook(ctx, rawPayload, signature); err != nil {
		logger.ErrorContext(ctx, "Error processing payment webhook", "error", err)

		errStr := err.Error()
		if strings.Contains(errStr, "webhook signature verification failed") ||
		   strings.Contains(errStr, "invalid signature") {
			http.Error(w, "Webhook signature verification failed", http.StatusBadRequest)
		} else if strings.Contains(errStr, "payment intent not found") {
            http.Error(w, "Payment intent not found", http.StatusNotFound)
        } else if strings.Contains(errStr, "already processed") {
            http.Error(w, "Webhook event already processed", http.StatusOK)
        } else {
			http.Error(w, "Internal server error processing webhook", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Webhook received successfully")); err != nil {
        logger.WarnContext(ctx, "Failed to write webhook success response", "error", err)
    }
	logger.InfoContext(ctx, "Payment webhook processed successfully")
}
