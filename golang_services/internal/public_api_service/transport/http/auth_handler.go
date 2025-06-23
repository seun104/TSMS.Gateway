package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
    "io"
	"strings" // For error checking in placeholder logic

	"github.com/AradIT/aradsms/golang_services/api/proto/userservice" // Corrected path
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/AradIT/aradsms/golang_services/internal/public_api_service/middleware"
    "github.com/go-playground/validator/v10" // For request validation
    "github.com/go-chi/chi/v5" // Added for RegisterRoutes type hint
)

// AuthHandler handles authentication related HTTP requests.
type AuthHandler struct {
	userServiceClient *grpc_clients.UserServiceClient
	logger            *slog.Logger
	validate          *validator.Validate // For request validation
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userServiceClient *grpc_clients.UserServiceClient, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		userServiceClient: userServiceClient,
		logger:            logger.With("handler", "auth"),
		validate:          validator.New(validator.WithRequiredStructEnabled()),
	}
}

// RegisterRoutes registers authentication routes with the given router.
func (h *AuthHandler) RegisterRoutes(r chi.Router) { // Assuming chi.Router is passed, or use http.ServeMux
	r.Post("/register", h.handleRegister)
	r.Post("/login", h.handleLogin)
	r.Post("/refresh_token", h.handleRefreshToken)
    // /users/me is typically in a user handler, but for simplicity in Phase 1, can be here.
    // It will be protected by the AuthMiddleware applied at the router group level.
}


func (h *AuthHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx) // Chi's middleware for request ID
	logger := h.logger.With("request_id", requestID)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        if errors.Is(err, io.EOF) {
			logger.WarnContext(ctx, "Empty request body for registration")
            h.jsonError(w, logger, "Request body is empty", http.StatusBadRequest) // Pass logger
            return
        }
		logger.ErrorContext(ctx, "Failed to decode registration request", "error", err)
		h.jsonError(w, logger, "Invalid request payload: "+err.Error(), http.StatusBadRequest) // Pass logger
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		logger.WarnContext(ctx, "Registration request validation failed", "error", err)
		h.jsonError(w, logger, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

    logger.InfoContext(ctx, "Registration attempt received", "username", req.Username, "email", req.Email)
    // Placeholder: Simulate call to a hypothetical RegisterUser RPC
    // _, err := h.userServiceClient.RegisterUser(r.Context(), &userservice.RegisterUserRequest{...})
    // For now, we can't call a RegisterUser RPC as it's not in auth.proto yet.
    // This handler is therefore incomplete until that gRPC endpoint exists.
    // For now, just log and return a placeholder success.

    // Let's assume RegisterUser RPC is added to userservice.AuthServiceInternal
    // and it returns a simple success/failure or user ID.
    // For now, we'll just return a generic message.
    // This needs to be updated once user_service.AuthServiceInternal has a Register method.

    // Simulate a successful registration for now for the structure.
    // This would actually call user_service's RegisterUser RPC.
    // Since that RPC doesn't exist in the current proto, this is conceptual.
    // A real implementation would need the proto updated and user_service implementing it.

    // For demonstration, let's construct a dummy success:
    // This logic should actually call the user_service via gRPC to register the user.
    // For now, we'll just return a placeholder.
    // This part of the code is a placeholder until the gRPC service `AuthServiceInternal`
    // in `user-service` has a `RegisterUser` method.

    // Let's assume a simplified Register via a generic gRPC call or a direct app call for now
    // This is a temporary simplification as RegisterUser RPC is not defined in current proto.
    // The actual call would be to h.userServiceClient which needs a Register method.
    // For the sake of progress on this handler, we'll assume such a method exists.
    // This will be a non-functional placeholder for the RPC call.
    // SimulateRegister and other Simulate methods would ideally use the passed-in logger for context.
    // For now, assuming they use their own or this is handled internally.
    _, err := h.userServiceClient.SimulateRegister(r.Context(), req.Username, req.Email, req.Password, req.FirstName, req.LastName, req.PhoneNumber)
    if err != nil {
        if strings.Contains(err.Error(), "exists") {
             h.jsonError(w, logger, err.Error(), http.StatusConflict) // Pass logger
        } else {
             h.jsonError(w, logger, "Registration failed: "+err.Error(), http.StatusInternalServerError) // Pass logger
        }
        return
    }


	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "User registered successfully. Please login."})
}

func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // Get context for logger and validation
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID) // Create handler-specific logger with request_id

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.ErrorContext(ctx, "Failed to decode login request", "error", err)
		h.jsonError(w, logger, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		logger.WarnContext(ctx, "Login request validation failed", "error", err)
		h.jsonError(w, logger, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// This will call user_service Login (which needs to be exposed via gRPC)
    // For now, user_service.AuthServiceInternal does not have a Login RPC.
    // This is a placeholder call.
    // We'll assume a Login RPC is added to userservice.AuthServiceInternal
    // that returns access_token, refresh_token, user_id, username.
    // This will require proto and user_service gRPC handler updates.

    // Placeholder for calling a Login RPC on UserServiceClient
    // loginResp, err := h.userServiceClient.Login(ctx, &userservice.LoginRequest{Username: req.Username, Password: req.Password})
    accessToken, refreshToken, userID, username, err := h.userServiceClient.SimulateLogin(ctx, req.Username, req.Password)
    if err != nil {
		logger.WarnContext(ctx, "Login simulation failed", "username", req.Username, "error", err)
        if strings.Contains(err.Error(), "invalid credentials") || strings.Contains(err.Error(), "not found") {
             h.jsonError(w, logger, "Invalid username or password", http.StatusUnauthorized)
        } else if strings.Contains(err.Error(), "not active") {
            h.jsonError(w, logger, "User account is not active", http.StatusForbidden)
        } else if strings.Contains(err.Error(), "locked") {
            h.jsonError(w, logger, "Account is temporarily locked", http.StatusForbidden)
        } else {
            h.jsonError(w, logger, "Login failed: "+err.Error(), http.StatusInternalServerError)
        }
        return
    }

	// Add UserID to logger context for subsequent logs if login is successful
	logger = logger.With("auth_user_id", userID)
	logger.InfoContext(ctx, "User login successful", "username", username)

	response := LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       userID,
		Username:     username,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AuthHandler) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := chi_middleware.GetReqID(ctx)
	logger := h.logger.With("request_id", requestID)

	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.ErrorContext(ctx, "Failed to decode refresh token request", "error", err)
		h.jsonError(w, logger, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if err := h.validate.StructCtx(ctx, req); err != nil {
		logger.WarnContext(ctx, "Refresh token request validation failed", "error", err)
		h.jsonError(w, logger, "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

    logger.InfoContext(ctx, "Refresh token attempt received")
    // Placeholder for calling a RefreshToken RPC on UserServiceClient
    // refreshResp, err := h.userServiceClient.RefreshToken(ctx, &userservice.RefreshTokenRequest{RefreshToken: req.RefreshToken})
    newAccessToken, newRefreshToken, err := h.userServiceClient.SimulateRefreshToken(ctx, req.RefreshToken)
    if err != nil {
		logger.WarnContext(ctx, "Token refresh simulation failed", "error", err)
        if strings.Contains(err.Error(), "invalid or expired") {
             h.jsonError(w, logger, "Invalid or expired refresh token", http.StatusUnauthorized)
        } else {
            h.jsonError(w, logger, "Token refresh failed: "+err.Error(), http.StatusInternalServerError)
        }
        return
    }

	logger.InfoContext(ctx, "Token refresh successful")
	response := LoginResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}


// jsonError is a helper to write JSON error responses.
// It now accepts a logger to ensure the request_id (if present in logger) is logged with the error.
// It uses GenericErrorResponse from the same package (dto.go).
func (h *AuthHandler) jsonError(w http.ResponseWriter, logger *slog.Logger, message string, statusCode int) {
	// The logger instance passed in should already have request_id if set by the handler.
	logger.Warn("API Error Response", "status_code", statusCode, "message", message)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(GenericErrorResponse{Error: message}) // Uses dto.GenericErrorResponse from the same package
}

// --- Add these to UserServiceClient in adapters/grpc_clients/user_service_client.go ---
// These are simulation methods until the gRPC service in user_service is updated.
// They help define the contract the UI handler expects.
/*
func (c *UserServiceClient) SimulateRegister(ctx context.Context, username, email, password, fname, lname, phone string) (string, error) {
    reqID, _ := ctx.Value(middleware.RequestIDKey).(string) // Example: How client might get reqID if propagated
    clientLogger := c.logger.With("request_id", reqID)
    clientLogger.InfoContext(ctx, "[SIMULATED] RegisterUser called in client", "username", username)
    // ...
}

func (c *UserServiceClient) SimulateLogin(ctx context.Context, username, password string) (string, string, string, string, error) {
    reqID, _ := ctx.Value(middleware.RequestIDKey).(string)
    clientLogger := c.logger.With("request_id", reqID)
    clientLogger.InfoContext(ctx, "[SIMULATED] LoginUser called in client", "username", username)
    // ...
}

func (c *UserServiceClient) SimulateRefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
    reqID, _ := ctx.Value(middleware.RequestIDKey).(string)
    clientLogger := c.logger.With("request_id", reqID)
    clientLogger.InfoContext(ctx, "[SIMULATED] RefreshToken called in client", "refreshToken", refreshToken)
    // ...
}
*/

// Import chi middleware for GetReqID
import chi_middleware "github.com/go-chi/chi/v5/middleware"
