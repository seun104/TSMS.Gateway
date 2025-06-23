package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aradsms/golang_services/api/proto/userservice"
	"github.com/aradsms/golang_services/internal/public_api_service/adapters/grpc_clients"
	"github.com/go-chi/chi/v5/middleware" // For GetReqID
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
func AuthMiddleware(userServiceClient *grpc_clients.UserServiceClient, baseLogger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetReqID(ctx)
			logger := baseLogger.With("request_id", requestID)

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.WarnContext(ctx, "Authorization header missing")
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 {
				logger.WarnContext(ctx, "Invalid Authorization header format", "header", authHeader)
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]
			var validatedUser *userservice.ValidatedUserResponse
			var err error

			if parts[0] == "Bearer" || parts[0] == "ApiKey" {
				validatedUser, err = userServiceClient.ValidateToken(ctx, tokenString)
			} else {
				logger.WarnContext(ctx, "Unsupported Authorization scheme", "scheme", parts[0])
				http.Error(w, "Unsupported Authorization scheme", http.StatusUnauthorized)
				return
			}

			if err != nil {
				logger.WarnContext(ctx, "Token validation failed", "error", err)
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			if validatedUser == nil || !validatedUser.IsActive {
				// Add UserID to log if validatedUser is not nil
				var logAttrs []slog.Attr
				if validatedUser != nil {
					logAttrs = append(logAttrs, slog.String("auth_user_id", validatedUser.GetUserId()))
				}
				logger.WarnContext(ctx, "Token valid but user inactive or validation response empty", logAttrs...)
				http.Error(w, "User account inactive or invalid", http.StatusForbidden)
				return
			}

			logger = logger.With(slog.String("auth_user_id", validatedUser.GetUserId()))
			logger.InfoContext(ctx, "User authenticated successfully")


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
func BasicPermissionCheckMiddleware(requiredPermission string, baseLogger *slog.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetReqID(ctx)
			logger := baseLogger.With("request_id", requestID)

            authUser, ok := ctx.Value(AuthenticatedUserContextKey).(AuthenticatedUser)
            if !ok {
                logger.ErrorContext(ctx,"AuthenticatedUser not found in context. AuthMiddleware must run first.")
                http.Error(w, "Internal server error", http.StatusInternalServerError)
                return
            }

            // Add UserID to logger for this check
            logger = logger.With("auth_user_id", authUser.ID)

            hasPermission := false
            for _, p := range authUser.Permissions {
                if p == requiredPermission {
                    hasPermission = true
                    break
                }
            }

            if !hasPermission {
                logger.WarnContext(ctx, "Permission denied",
                    "required_permission", requiredPermission,
                    "user_permissions", strings.Join(authUser.Permissions, ","))
                http.Error(w, "Forbidden: You don't have permission to perform this action.", http.StatusForbidden)
                return
            }
            logger.DebugContext(ctx, "Permission granted", "required_permission", requiredPermission)
            next.ServeHTTP(w, r)
        })
    }
}
