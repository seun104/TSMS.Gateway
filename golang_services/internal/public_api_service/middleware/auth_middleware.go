package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aradsms/golang_services/api/proto/userservice" // Adjust to your go.mod path
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	AuthenticatedUserContextKey = ContextKey("authenticatedUser")
)

// AuthenticatedUser holds information about the authenticated user.
type AuthenticatedUser struct {
	ID          string
	Username    string
	RoleID      string
	IsAdmin     bool
	Permissions []string
    IsActive    bool
}

// AuthMiddleware creates a middleware for authenticating requests.
func AuthMiddleware(userServiceClient *grpc_clients.UserServiceClient, logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.WarnContext(r.Context(), "Authorization header missing")
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 {
				logger.WarnContext(r.Context(), "Invalid Authorization header format", "header", authHeader)
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]
			var validatedUser *userservice.ValidatedUserResponse
			var err error

			// Could check parts[0] for "Bearer" vs "ApiKey" here if supporting both via same header
			// For now, assume token can be either JWT or API Key, validated by user_service
			if parts[0] == "Bearer" || parts[0] == "ApiKey" { // Allow both schemes
				validatedUser, err = userServiceClient.ValidateToken(r.Context(), tokenString)
			} else {
				logger.WarnContext(r.Context(), "Unsupported Authorization scheme", "scheme", parts[0])
				http.Error(w, "Unsupported Authorization scheme", http.StatusUnauthorized)
				return
			}


			if err != nil {
				logger.WarnContext(r.Context(), "Token validation failed", "error", err)
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized) // Generic error
				return
			}

			if validatedUser == nil || !validatedUser.IsActive {
				logger.WarnContext(r.Context(), "Token valid but user inactive or validation response empty", "userID", validatedUser.GetUserId())
				http.Error(w, "User account inactive or invalid", http.StatusForbidden)
				return
			}

			authUser := AuthenticatedUser{
				ID:          validatedUser.UserId,
				Username:    validatedUser.Username,
				RoleID:      validatedUser.RoleId,
				IsAdmin:     validatedUser.IsAdmin,
				Permissions: validatedUser.Permissions,
                IsActive:    validatedUser.IsActive,
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), AuthenticatedUserContextKey, authUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// BasicPermissionCheckMiddleware checks if the authenticated user has a specific permission.
// This is a very basic example. More complex RBAC/ABAC might use a library like Casbin.
func BasicPermissionCheckMiddleware(requiredPermission string, logger *slog.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authUser, ok := r.Context().Value(AuthenticatedUserContextKey).(AuthenticatedUser)
            if !ok {
                logger.ErrorContext(r.Context(),"AuthenticatedUser not found in context. AuthMiddleware must run first.")
                http.Error(w, "Internal server error", http.StatusInternalServerError)
                return
            }

            hasPermission := false
            for _, p := range authUser.Permissions {
                if p == requiredPermission {
                    hasPermission = true
                    break
                }
            }

            if !hasPermission {
                logger.WarnContext(r.Context(), "Permission denied",
                    "userID", authUser.ID,
                    "required_permission", requiredPermission,
                    "user_permissions", strings.Join(authUser.Permissions, ","))
                http.Error(w, "Forbidden: You don't have permission to perform this action.", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
