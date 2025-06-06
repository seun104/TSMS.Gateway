package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"log/slog"
)

type MagfaSMSProvider struct {
	logger     *slog.Logger
	httpClient *http.Client
	apiUrl    string
	apiKey    string
	senderId  string
}

func NewMagfaSMSProvider(logger *slog.Logger, apiUrl, apiKey, senderId string, httpClient *http.Client) *MagfaSMSProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &MagfaSMSProvider{
		logger:     logger.With("provider", "magfa"),
		httpClient: httpClient,
		apiUrl:    apiUrl,
		apiKey:    apiKey,
		senderId:  senderId,
	}
}

// MagfaSendRequestBody defines the structure for Magfa's send SMS API.
// Note: This is based on a common Magfa structure, actual fields might vary.
// Usually it's an array of messages.
type MagfaSendRequestBody struct {
	Messages    []MagfaMessage `json:"messages"`
	Senders     []string       `json:"senders,omitempty"`    // Optional: If using a pool or specific senders for a batch
	Encodings   []string       `json:"encodings,omitempty"`  // Optional: e.g., "utf-8"
	Udh         string         `json:"udh,omitempty"`        // Optional: For concatenated SMS
}

type MagfaMessage struct {
	Sender    string   `json:"sender"`
	Body      string   `json:"body"`
	Recipients []string `json:"recipients"` // Magfa typically supports multiple recipients per message body
}


// MagfaSendSuccessResponse defines a potential success response structure.
type MagfaSendSuccessResponse struct {
    Messages []MagfaSentMessageDetail `json:"messages"`
    Status   int                      `json:"status"` // Typically a status code like 0 or 1 for overall success
    Message  string                   `json:"message"` // Optional: success message
}

type MagfaSentMessageDetail struct {
    ID         int64  `json:"id"`          // Magfa's message ID
    Recipient  string `json:"recipient"`   // Recipient number
    Status     int    `json:"status"`      // Status for this specific message
    Message    string `json:"message"`     // Optional message for this recipient
}


// MagfaErrorResponse defines a potential error response structure.
type MagfaErrorResponse struct {
	Status  int    `json:"status"`  // HTTP status or Magfa error code
	Message string `json:"message"` // Error message
	// Errors  []string `json:"errors,omitempty"` // Optional: more detailed errors
}


func (p *MagfaSMSProvider) Send(ctx context.Context, details SendRequestDetails) (*SendResponseDetails, error) {
	p.logger.InfoContext(ctx, "MagfaSMSProvider: Send called", "recipient", details.Recipient, "internal_message_id", details.InternalMessageID)

	// Magfa typically expects an array of recipients for a single message body
	magfaReqBody := MagfaSendRequestBody{
		Messages: []MagfaMessage{
			{
				Sender:    p.senderId,
				Body:      details.Content,
				Recipients: []string{details.Recipient}, // Send to one recipient per this call
			},
		},
	}

	reqBytes, err := json.Marshal(magfaReqBody)
	if err != nil {
		p.logger.ErrorContext(ctx, "Failed to marshal Magfa request", "error", err, "internal_message_id", details.InternalMessageID)
		return nil, fmt.Errorf("failed to marshal request for Magfa: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiUrl, bytes.NewBuffer(reqBytes))
	if err != nil {
		p.logger.ErrorContext(ctx, "Failed to create Magfa HTTP request", "error", err, "internal_message_id", details.InternalMessageID)
		return nil, fmt.Errorf("failed to create HTTP request for Magfa: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Magfa authentication can vary: API Key in header, Basic Auth, or token.
	// This example uses a common "Authorization: ApiKey YOUR_API_KEY" or "Bearer YOUR_API_KEY"
	// Adjust if Magfa uses a different scheme like `httpReq.SetBasicAuth(username, password)`
	// Or if apiKey is part of the JSON body itself.
	// The problem description implies a Bearer token style.
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	p.logger.DebugContext(ctx, "Sending HTTP request to Magfa", "url", p.apiUrl, "body", string(reqBytes))

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.logger.ErrorContext(ctx, "Failed to send request to Magfa", "error", err, "internal_message_id", details.InternalMessageID)
		return nil, fmt.Errorf("failed to send request to Magfa: %w", err)
	}
	defer httpResp.Body.Close()

	respBodyBytes, readErr := io.ReadAll(httpResp.Body)
    if readErr != nil {
        p.logger.ErrorContext(ctx, "Failed to read Magfa response body", "status_code", httpResp.StatusCode, "error", readErr, "internal_message_id", details.InternalMessageID)
        return &SendResponseDetails{
			IsSuccess:      false,
			ProviderStatus: fmt.Sprintf("FAILED_MAGFA_READ_ERR_%d", httpResp.StatusCode),
			ErrorMessage:   fmt.Sprintf("Magfa API request failed (status %d), and failed to read response body: %v", httpResp.StatusCode, readErr),
		}, fmt.Errorf("Magfa API request failed (status %d), and failed to read response body: %v", httpResp.StatusCode, readErr)
    }
	p.logger.DebugContext(ctx, "Received HTTP response from Magfa", "status_code", httpResp.StatusCode, "body", string(respBodyBytes))


	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		// Attempt to parse Magfa's specific success response
		var magfaSuccessResp MagfaSendSuccessResponse
		if err := json.Unmarshal(respBodyBytes, &magfaSuccessResp); err != nil {
			// If parsing fails, it might be a non-JSON success response or a different structure
			p.logger.WarnContext(ctx, "Successfully sent to Magfa, but failed to parse success response body", "status_code", httpResp.StatusCode, "error", err, "body", string(respBodyBytes), "internal_message_id", details.InternalMessageID)
			// Even if parsing fails, it's a success from HTTP perspective.
			// ProviderMessageID might be unavailable.
			return &SendResponseDetails{
				IsSuccess:         true,
				ProviderMessageID: "", // Or a default/derived ID if possible
				ProviderStatus:    fmt.Sprintf("SENT_MAGFA_%d_UNPARSED_RESP", httpResp.StatusCode),
			}, nil
		}

		providerMsgID := ""
		// Assuming the first message in the response corresponds to our request
		if len(magfaSuccessResp.Messages) > 0 {
			providerMsgID = fmt.Sprintf("%d", magfaSuccessResp.Messages[0].ID) // Magfa ID is often numeric
		}

		p.logger.InfoContext(ctx, "Successfully sent SMS via Magfa", "status_code", httpResp.StatusCode, "provider_message_id", providerMsgID, "internal_message_id", details.InternalMessageID)
		return &SendResponseDetails{
			ProviderMessageID: providerMsgID,
			IsSuccess:         true,
			ProviderStatus:    fmt.Sprintf("SENT_MAGFA_%d", httpResp.StatusCode),
		}, nil
	} else {
		// Attempt to parse Magfa's specific error response
		var magfaErrorResp MagfaErrorResponse
		errMsg := fmt.Sprintf("Magfa API error: status %d", httpResp.StatusCode)

		if err := json.Unmarshal(respBodyBytes, &magfaErrorResp); err == nil {
			if magfaErrorResp.Message != "" {
				errMsg = fmt.Sprintf("Magfa API error: status %d, message: %s", httpResp.StatusCode, magfaErrorResp.Message)
			}
		} else {
			p.logger.WarnContext(ctx, "Magfa send failed, and failed to parse error response body", "status_code", httpResp.StatusCode, "error", err, "body", string(respBodyBytes), "internal_message_id", details.InternalMessageID)
			// Use the raw body or a generic error message if parsing fails
			if len(respBodyBytes) > 0 && len(respBodyBytes) < 200 { // Avoid logging huge bodies
                 errMsg = fmt.Sprintf("Magfa API error: status %d, raw_body: %s", httpResp.StatusCode, string(respBodyBytes))
            }
		}

		p.logger.WarnContext(ctx, "Magfa send failed", "status_code", httpResp.StatusCode, "parsed_error_message", magfaErrorResp.Message, "final_error_message", errMsg, "internal_message_id", details.InternalMessageID)
		return &SendResponseDetails{
			IsSuccess:      false,
			ProviderStatus: fmt.Sprintf("FAILED_MAGFA_%d", httpResp.StatusCode),
			ErrorMessage:   errMsg,
		}, fmt.Errorf(errMsg) // Return the error to the caller as well
	}
}

func (p *MagfaSMSProvider) GetName() string {
	return "magfa" // Consistent name, e.g., for map keys
}
