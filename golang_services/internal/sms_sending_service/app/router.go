package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aradsms/golang_services/internal/sms_sending_service/domain" // Corrected path
	"github.com/aradsms/golang_services/internal/sms_sending_service/provider" // Corrected path
	"github.com/google/uuid"
)

// Router is responsible for selecting an SMS provider based on defined routes.
type Router struct {
	routeRepo domain.RouteRepository
	providers map[string]provider.SMSSenderProvider // Keyed by provider name (e.g., "magfa", "mock")
	logger    *slog.Logger
}

// NewRouter creates a new Router.
func NewRouter(routeRepo domain.RouteRepository, providers map[string]provider.SMSSenderProvider, logger *slog.Logger) *Router {
	return &Router{
		routeRepo: routeRepo,
		providers: providers,
		logger:    logger.With("component", "router"),
	}
}

// SelectProvider selects an appropriate SMS provider based on the recipient and userID.
// It returns the selected provider or nil if no specific route matches.
// An error is returned if fetching routes fails.
func (r *Router) SelectProvider(ctx context.Context, recipient string, userIDStr string) (provider.SMSSenderProvider, error) {
	r.logger.DebugContext(ctx, "Attempting to select provider", "recipient", recipient, "userID", userIDStr)
	routes, err := r.routeRepo.GetActiveRoutesOrderedByPriority(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get active routes for provider selection", "error", err)
		return nil, fmt.Errorf("failed to get active routes: %w", err)
	}

	if len(routes) == 0 {
		r.logger.InfoContext(ctx, "No active routes configured.")
		return nil, nil // No routes configured, so no specific provider selected by router
	}

	var parsedUserID uuid.UUID
	if userIDStr != "" {
		parsedUserID, err = uuid.Parse(userIDStr)
		if err != nil {
			r.logger.WarnContext(ctx, "Invalid userID string, cannot use for UserID criteria matching", "userID", userIDStr, "error", err)
			// Continue without parsedUserID, UserID criteria requiring a valid UUID won't match.
		}
	}


	for _, route := range routes {
		r.logger.DebugContext(ctx, "Evaluating route", "route_name", route.Name, "route_priority", route.Priority, "route_criteria", route.CriteriaJSON)
		if r.matches(recipient, parsedUserID, route.Criteria) {
			providerInstance, ok := r.providers[route.SmsProviderName]
			if !ok {
				r.logger.WarnContext(ctx, "Route matched, but provider instance not found in available providers map",
					"route_id", route.ID, "route_name", route.Name, "provider_name_from_route", route.SmsProviderName)
				continue // Try next route
			}
			r.logger.InfoContext(ctx, "Route matched and provider found",
				"route_name", route.Name, "provider_name", providerInstance.GetName(), "recipient", recipient, "userID", userIDStr)
			return providerInstance, nil
		}
	}

	r.logger.InfoContext(ctx, "No specific route matched for recipient criteria", "recipient", recipient, "userID", userIDStr)
	return nil, nil // No specific route matched
}

// matches checks if the given recipient and userID match the route's criteria.
func (r *Router) matches(recipient string, userID uuid.UUID, criteria domain.RouteCriteria) bool {
	// CountryCode matching
	if criteria.CountryCode != nil && *criteria.CountryCode != "" {
		normalizedRecipient := strings.TrimPrefix(recipient, "+")
		if !strings.HasPrefix(normalizedRecipient, *criteria.CountryCode) {
			r.logger.Debug("CountryCode mismatch", "recipient", recipient, "criteria_code", *criteria.CountryCode)
			return false
		}
	}

	// OperatorPrefix matching
	if criteria.OperatorPrefix != nil && *criteria.OperatorPrefix != "" {
		// Strip country code (if specified in criteria) before checking prefix
		tempRecipient := strings.TrimPrefix(recipient, "+")
		if criteria.CountryCode != nil && *criteria.CountryCode != "" {
			if strings.HasPrefix(tempRecipient, *criteria.CountryCode) {
				tempRecipient = strings.TrimPrefix(tempRecipient, *criteria.CountryCode)
			} else {
				// If CountryCode criteria is set but doesn't match recipient, this rule shouldn't apply anyway,
				// but this check is for operator prefix, so if country code didn't match, prefix match is irrelevant.
				// This state should ideally be caught by CountryCode check if it was present.
			}
		}

		if !strings.HasPrefix(tempRecipient, *criteria.OperatorPrefix) {
			r.logger.Debug("OperatorPrefix mismatch", "recipient_part_for_prefix", tempRecipient, "criteria_prefix", *criteria.OperatorPrefix)
			return false
		}
	}

	// UserID matching
	if criteria.UserID != nil && *criteria.UserID != "" {
		if userID == uuid.Nil { // UserID from message is not a valid UUID or not provided
			r.logger.Debug("UserID criteria set, but message UserID is nil/invalid", "criteria_userID", *criteria.UserID)
			return false
		}
		if userID.String() != *criteria.UserID {
			r.logger.Debug("UserID mismatch", "message_userID", userID.String(), "criteria_userID", *criteria.UserID)
			return false
		}
	}

	// If all present criteria matched, return true
	r.logger.Debug("All criteria matched for route")
	return true
}
