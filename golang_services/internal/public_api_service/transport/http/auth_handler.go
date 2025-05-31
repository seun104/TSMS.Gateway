package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
    "io"


	"github.com/aradsms/golang_services/api/proto/userservice" // Adjust to your go.mod path
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/aradsms/golang_services/internal/public_api_service/middleware"
    // "github.com/go-playground/validator/v10" // For request validation
    "github.com/go-chi/chi/v5" // Added for RegisterRoutes type hint
)

// AuthHandler handles authentication related HTTP requests.
type AuthHandler struct {
	userServiceClient *grpc_clients.UserServiceClient
	logger            *slog.Logger
	// validate          *validator.Validate // For request validation
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userServiceClient *grpc_clients.UserServiceClient, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		userServiceClient: userServiceClient,
		logger:            logger.With("handler", "auth"),
		// validate:          validator.New(validator.WithRequiredStructEnabled()),
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
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        if errors.Is(err, io.EOF) {
            h.jsonError(w, "Request body is empty", http.StatusBadRequest)
            return
        }
		h.jsonError(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}
    // TODO: Add request validation using 'validate' tag on RegisterRequest struct
    // if err := h.validate.Struct(req); err != nil {
    //     h.jsonError(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
    //     return
    // }


	// This gRPC call needs to be implemented in user_service's gRPC server
    // For now, let's assume it takes these parameters and returns a user ID or error.
    // The user_service.AuthServiceInternalClient needs a RegisterUser method.
    // Let's assume we add a Register RPC to auth.proto and implement it in user_service.
    // For now, we'll simulate this and focus on the API handler structure.
    // This part will require changes once the gRPC service in user_service is updated.

    h.logger.InfoContext(r.Context(), "Registration attempt", "username", req.Username, "email", req.Email)
    // Placeholder: Simulate call to a hypothetical RegisterUser RPC
    // In reality, you'd call something like:
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
    _, err := h.userServiceClient.SimulateRegister(r.Context(), req.Username, req.Email, req.Password, req.FirstName, req.LastName, req.PhoneNumber)
    if err != nil {
        // Map gRPC errors to HTTP errors
        // e.g. if status.Code(err) == codes.AlreadyExists
        if strings.Contains(err.Error(), "exists") { // Basic error check
             h.jsonError(w, err.Error(), http.StatusConflict)
        } else {
             h.jsonError(w, "Registration failed: "+err.Error(), http.StatusInternalServerError)
        }
        return
    }


	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "User registered successfully. Please login."})
}

func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
    // TODO: Add request validation

	// This will call user_service Login (which needs to be exposed via gRPC)
    // For now, user_service.AuthServiceInternal does not have a Login RPC.
    // This is a placeholder call.
    // We'll assume a Login RPC is added to userservice.AuthServiceInternal
    // that returns access_token, refresh_token, user_id, username.
    // This will require proto and user_service gRPC handler updates.

    // Placeholder for calling a Login RPC on UserServiceClient
    // loginResp, err := h.userServiceClient.Login(r.Context(), &userservice.LoginRequest{Username: req.Username, Password: req.Password})
    // For demonstration, let's construct a dummy success:
    // This logic should call user_service via gRPC.
    // We will simulate the call for now.
    accessToken, refreshToken, userID, username, err := h.userServiceClient.SimulateLogin(r.Context(), req.Username, req.Password)
    if err != nil {
        if strings.Contains(err.Error(), "invalid credentials") || strings.Contains(err.Error(), "not found") {
             h.jsonError(w, "Invalid username or password", http.StatusUnauthorized)
        } else if strings.Contains(err.Error(), "not active") {
            h.jsonError(w, "User account is not active", http.StatusForbidden)
        } else if strings.Contains(err.Error(), "locked") {
            h.jsonError(w, "Account is temporarily locked", http.StatusForbidden)
        } else {
            h.jsonError(w, "Login failed: "+err.Error(), http.StatusInternalServerError)
        }
        return
    }


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
	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
    // TODO: Add request validation

    // This will call user_service RefreshToken (which needs to be exposed via gRPC)
    // Placeholder for calling a RefreshToken RPC on UserServiceClient
    // refreshResp, err := h.userServiceClient.RefreshToken(r.Context(), &userservice.RefreshTokenRequest{RefreshToken: req.RefreshToken})
    // For demonstration:
    newAccessToken, newRefreshToken, err := h.userServiceClient.SimulateRefreshToken(r.Context(), req.RefreshToken)
    if err != nil {
        if strings.Contains(err.Error(), "invalid or expired") {
             h.jsonError(w, "Invalid or expired refresh token", http.StatusUnauthorized)
        } else {
            h.jsonError(w, "Token refresh failed: "+err.Error(), http.StatusInternalServerError)
        }
        return
    }

	response := LoginResponse{ // Reuses LoginResponse for refreshed tokens
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
        // UserID and Username might not be needed/returned on refresh, depends on design
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}


// jsonError is a helper to write JSON error responses.
func (h *AuthHandler) jsonError(w http.ResponseWriter, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(GenericErrorResponse{Error: message})
}

// --- Add these to UserServiceClient in adapters/grpc_clients/user_service_client.go ---
// These are simulation methods until the gRPC service in user_service is updated.
// They help define the contract the UI handler expects.
/*
func (c *UserServiceClient) SimulateRegister(ctx context.Context, username, email, password, fname, lname, phone string) (string, error) {
    c.logger.InfoContext(ctx, "[SIMULATED] RegisterUser called in client", "username", username)
    // In a real scenario, this would make a gRPC call.
    // Simulate some basic validation or error conditions based on input
    if username == "exists" {
        return "", errors.New("username already exists")
    }
    if email == "exists@example.com" {
        return "", errors.New("email already exists")
    }
    return "simulated-user-id-" + username, nil
}

func (c *UserServiceClient) SimulateLogin(ctx context.Context, username, password string) (string, string, string, string, error) {
    c.logger.InfoContext(ctx, "[SIMULATED] LoginUser called in client", "username", username)
    if username == "testuser" && password == "password" {
        return "simulated-access-token", "simulated-refresh-token", "user-id-123", username, nil
    }
    if username == "inactive" && password == "password" {
        return "", "", "", "", errors.New("user account is not active")
    }
    if username == "locked" && password == "password" {
        return "", "", "", "", errors.New("account is temporarily locked")
    }
    return "", "", "", "", errors.New("invalid credentials")
}

func (c *UserServiceClient) SimulateRefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
    c.logger.InfoContext(ctx, "[SIMULATED] RefreshToken called in client", "refreshToken", refreshToken)
    if refreshToken == "valid-refresh-token" {
        return "new-simulated-access-token", "new-simulated-refresh-token", nil
    }
    return "", "", errors.New("invalid or expired refresh token")
}
*/
