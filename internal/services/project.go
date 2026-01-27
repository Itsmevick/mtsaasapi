package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/models"
)

type ProjectService struct {
	db *pgxpool.Pool
}

func NewProjectService(db *pgxpool.Pool) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) Create(ctx context.Context, workspaceID uuid.UUID, name, description string) (*models.Project, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	var project models.Project
	err = s.db.QueryRow(ctx,
		`INSERT INTO projects (workspace_id, name, description) 
		 VALUES ($1, $2, $3) 
		 RETURNING id, workspace_id, name, description, deleted_at, created_at, updated_at`,
		workspaceID, name, description,
	).Scan(
		&project.ID, &project.WorkspaceID, &project.Name, &project.Description,
		&project.DeletedAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (s *ProjectService) GetByID(ctx context.Context, workspaceID, projectID uuid.UUID) (*models.Project, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	var project models.Project
	err = s.db.QueryRow(ctx,
		`SELECT id, workspace_id, name, description, deleted_at, created_at, updated_at 
		 FROM projects 
		 WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL`,
		projectID, workspaceID,
	).Scan(
		&project.ID, &project.WorkspaceID, &project.Name, &project.Description,
		&project.DeletedAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("project not found")
	}
	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (s *ProjectService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.Project, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, workspace_id, name, description, deleted_at, created_at, updated_at 
		 FROM projects 
		 WHERE workspace_id = $1 AND deleted_at IS NULL 
		 ORDER BY created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		err := rows.Scan(
			&p.ID, &p.WorkspaceID, &p.Name, &p.Description,
			&p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}

	return projects, nil
}

func (s *ProjectService) Update(ctx context.Context, workspaceID, projectID uuid.UUID, name, description string) (*models.Project, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	var project models.Project
	err = s.db.QueryRow(ctx,
		`UPDATE projects SET name = $1, description = $2, updated_at = NOW() 
		 WHERE id = $3 AND workspace_id = $4 AND deleted_at IS NULL 
		 RETURNING id, workspace_id, name, description, deleted_at, created_at, updated_at`,
		name, description, projectID, workspaceID,
	).Scan(
		&project.ID, &project.WorkspaceID, &project.Name, &project.Description,
		&project.DeletedAt, &project.CreatedAt, &project.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("project not found")
	}
	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (s *ProjectService) Delete(ctx context.Context, workspaceID, projectID uuid.UUID) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	// Soft delete
	now := time.Now()
	_, err = s.db.Exec(ctx,
		`UPDATE projects SET deleted_at = $1, updated_at = NOW() 
		 WHERE id = $2 AND workspace_id = $3 AND deleted_at IS NULL`,
		now, projectID, workspaceID,
	)
	return err
}
