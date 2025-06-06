package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5" // For pgx.ErrNoRows if needed, though Query usually doesn't return it directly
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"

	"github.com/aradsms/golang_services/internal/sms_sending_service/domain" // Corrected path
)

type PgRouteRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewPgRouteRepository creates a new PostgreSQL route repository.
func NewPgRouteRepository(db *pgxpool.Pool, logger *slog.Logger) domain.RouteRepository { // Return interface
	return &PgRouteRepository{db: db, logger: logger.With("component", "route_repository_pg")}
}

func (r *PgRouteRepository) GetActiveRoutesOrderedByPriority(ctx context.Context) ([]*domain.Route, error) {
	query := `
		SELECT r.id, r.name, r.priority, r.criteria_json, r.sms_provider_id, sp.name as sms_provider_name,
		       r.is_active, r.created_at, r.updated_at
		FROM routes r
		JOIN sms_providers sp ON r.sms_provider_id = sp.id
		WHERE r.is_active = TRUE
		ORDER BY r.priority ASC, r.created_at DESC
	` // Added created_at for stable sort on same priority

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error querying active routes", "error", err)
		return nil, fmt.Errorf("querying active routes: %w", err)
	}
	defer rows.Close()

	var routes []*domain.Route
	for rows.Next() {
		var route domain.Route
		var criteriaJSONString string

		if err := rows.Scan(
			&route.ID, &route.Name, &route.Priority, &criteriaJSONString,
			&route.SmsProviderID,
			&route.SmsProviderName,
			&route.IsActive, &route.CreatedAt, &route.UpdatedAt,
		); err != nil {
			r.logger.ErrorContext(ctx, "Error scanning route row", "error", err)
			// Decide if one bad row should stop all routing. For now, skip.
			continue
		}

		route.CriteriaJSON = criteriaJSONString
		if criteriaJSONString != "" && criteriaJSONString != "{}" { // Avoid unmarshal error on empty/null JSON
			if err := json.Unmarshal([]byte(criteriaJSONString), &route.Criteria); err != nil {
				r.logger.ErrorContext(ctx, "Error unmarshalling route criteria_json", "route_id", route.ID, "json_string", criteriaJSONString, "error", err)
				// If criteria is critical and unparseable, skip this route
				continue
			}
		}
		routes = append(routes, &route)
	}

	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error after iterating route rows", "error", err)
		return nil, fmt.Errorf("iterating route rows: %w", err)
	}

	r.logger.InfoContext(ctx, "Fetched active routes", "count", len(routes))
	return routes, nil
}
