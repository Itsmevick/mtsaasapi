package services

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/models"
)

type ActivityService struct {
	db *pgxpool.Pool
}

func NewActivityService(db *pgxpool.Pool) *ActivityService {
	return &ActivityService{db: db}
}

func (s *ActivityService) Log(ctx context.Context, workspaceID, userID uuid.UUID, action, entityType string, entityID uuid.UUID, metadata map[string]interface{}) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return nil // Silently fail if tenant mismatch
	}

	var metadataJSON json.RawMessage
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO activity_logs (workspace_id, user_id, action, entity_type, entity_id, metadata) 
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		workspaceID, userID, action, entityType, entityID, metadataJSON,
	)
	return err
}

func (s *ActivityService) List(ctx context.Context, workspaceID uuid.UUID, limit int) ([]models.ActivityLog, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, err
	}

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, workspace_id, user_id, action, entity_type, entity_id, metadata, created_at 
		 FROM activity_logs 
		 WHERE workspace_id = $1 
		 ORDER BY created_at DESC 
		 LIMIT $2`,
		workspaceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []models.ActivityLog
	for rows.Next() {
		var a models.ActivityLog
		err := rows.Scan(
			&a.ID, &a.WorkspaceID, &a.UserID, &a.Action,
			&a.EntityType, &a.EntityID, &a.Metadata, &a.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}

	return activities, nil
}
