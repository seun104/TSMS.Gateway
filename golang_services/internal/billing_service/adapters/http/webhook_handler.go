package http

import (
	"context"
	"io"
	"net/http"
	"log/slog"
	"strings" // For error message checking

	// Corrected import for billing app service (assuming BillingAppService struct is in 'app' package)
	"github.com/AradIT/aradsms/golang_services/internal/billing_service/app"
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
	if r.Method != http.MethodPost {
		h.logger.WarnContext(ctx, "Method not allowed for webhook", "method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Example: Extract a signature header. For Stripe, it's "Stripe-Signature".
	// For a generic mock, this might be optional or a fixed value.
	signature := r.Header.Get("X-Payment-Signature") // Using a generic header for now; make configurable per gateway

	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	rawPayload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to read webhook request body", "error", err)
		if err.Error() == "http: request body too large" { // Check exact error string if possible
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
		}
		return
	}
	// It's good practice to close r.Body, but io.ReadAll does it if it reads to EOF.
	// If there was an error, it might not have, so defer r.Body.Close() can be added before ReadAll.
	// However, for MaxBytesReader, the original body is replaced.
	// defer r.Body.Close() // Not strictly needed after ReadAll with MaxBytesReader wrapper, but harmless.

	h.logger.InfoContext(ctx, "Received payment webhook",
		"remote_addr", r.RemoteAddr,
		"content_length", r.ContentLength, // ContentLength from header, actual might differ
		"payload_size", len(rawPayload),
		"signature_present", signature != "")

	if err := h.appService.HandlePaymentWebhook(ctx, rawPayload, signature); err != nil {
		h.logger.ErrorContext(ctx, "Error processing payment webhook", "error", err)

		// Check for specific error types or messages to return appropriate HTTP status codes
		// This uses string comparison, which is fragile. Ideally, use custom error types/wrapping.
		errStr := err.Error()
		if strings.Contains(errStr, "webhook signature verification failed") ||
		   strings.Contains(errStr, "invalid signature") { // Example error messages from adapter
			http.Error(w, "Webhook signature verification failed", http.StatusBadRequest)
		} else if strings.Contains(errStr, "payment intent not found") { // Example from app service
            http.Error(w, "Payment intent not found", http.StatusNotFound) // Or StatusBadRequest if the ID was malformed
        } else if strings.Contains(errStr, "already processed") { // Example for idempotency
            http.Error(w, "Webhook event already processed", http.StatusOK) // Or 202 Accepted
        } else {
			http.Error(w, "Internal server error processing webhook", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Webhook received successfully")); err != nil {
        h.logger.WarnContext(ctx, "Failed to write webhook success response", "error", err)
    }
	h.logger.InfoContext(ctx, "Payment webhook processed successfully")
}
