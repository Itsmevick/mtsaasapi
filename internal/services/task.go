package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/models"
)

type TaskService struct {
	db *pgxpool.Pool
}

func NewTaskService(db *pgxpool.Pool) *TaskService {
	return &TaskService{db: db}
}

func (s *TaskService) Create(ctx context.Context, workspaceID uuid.UUID, projectID *uuid.UUID, title, description, status string) (*models.Task, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	// Verify project belongs to workspace if provided
	if projectID != nil {
		var projectWorkspaceID uuid.UUID
		err = s.db.QueryRow(ctx,
			"SELECT workspace_id FROM projects WHERE id = $1 AND deleted_at IS NULL",
			*projectID,
		).Scan(&projectWorkspaceID)
		if err == pgx.ErrNoRows {
			return nil, errors.New("project not found")
		}
		if err != nil {
			return nil, err
		}
		if projectWorkspaceID != workspaceID {
			return nil, errors.New("project does not belong to workspace")
		}
	}

	if status == "" {
		status = "todo"
	}

	// Map frontend status to backend status
	backendStatus := status
	if status == "doing" {
		backendStatus = "in_progress"
	}

	var task models.Task
	if projectID != nil {
		err = s.db.QueryRow(ctx,
			`INSERT INTO tasks (workspace_id, project_id, title, description, status) 
			 VALUES ($1, $2, $3, $4, $5) 
			 RETURNING id, workspace_id, project_id, title, description, status, deleted_at, created_at, updated_at`,
			workspaceID, *projectID, title, description, backendStatus,
		).Scan(
			&task.ID, &task.WorkspaceID, &task.ProjectID, &task.Title,
			&task.Description, &task.Status, &task.DeletedAt,
			&task.CreatedAt, &task.UpdatedAt,
		)
	} else {
		// Handle NULL project_id
		err = s.db.QueryRow(ctx,
			`INSERT INTO tasks (workspace_id, project_id, title, description, status) 
			 VALUES ($1, NULL, $2, $3, $4) 
			 RETURNING id, workspace_id, project_id, title, description, status, deleted_at, created_at, updated_at`,
			workspaceID, title, description, backendStatus,
		).Scan(
			&task.ID, &task.WorkspaceID, &task.ProjectID, &task.Title,
			&task.Description, &task.Status, &task.DeletedAt,
			&task.CreatedAt, &task.UpdatedAt,
		)
	}
	if err != nil {
		return nil, err
	}

	// Map backend status to frontend status
	if task.Status == "in_progress" {
		task.Status = "doing"
	}

	return &task, nil
}

func (s *TaskService) GetByID(ctx context.Context, workspaceID, taskID uuid.UUID) (*models.Task, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	var task models.Task
	err = s.db.QueryRow(ctx,
		`SELECT id, workspace_id, project_id, title, description, status, deleted_at, created_at, updated_at 
		 FROM tasks 
		 WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL`,
		taskID, workspaceID,
	).Scan(
		&task.ID, &task.WorkspaceID, &task.ProjectID, &task.Title,
		&task.Description, &task.Status, &task.DeletedAt,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("task not found")
	}
	if err != nil {
		return nil, err
	}

	// Map backend status to frontend status
	if task.Status == "in_progress" {
		task.Status = "doing"
	}

	return &task, nil
}

func (s *TaskService) List(ctx context.Context, workspaceID uuid.UUID, projectID *uuid.UUID, statusFilter string) ([]models.Task, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	query := `SELECT id, workspace_id, project_id, title, description, status, deleted_at, created_at, updated_at 
			  FROM tasks 
			  WHERE workspace_id = $1 AND deleted_at IS NULL`
	args := []interface{}{workspaceID}
	argIndex := 2

	if projectID != nil {
		query += ` AND project_id = $` + fmt.Sprintf("%d", argIndex)
		args = append(args, *projectID)
		argIndex++
	}

	if statusFilter != "" {
		// Map frontend status to backend status
		backendStatus := statusFilter
		if statusFilter == "doing" {
			backendStatus = "in_progress"
		}
		query += ` AND status = $` + fmt.Sprintf("%d", argIndex)
		args = append(args, backendStatus)
		argIndex++
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		err := rows.Scan(
			&t.ID, &t.WorkspaceID, &t.ProjectID, &t.Title,
			&t.Description, &t.Status, &t.DeletedAt,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		// Map backend status to frontend status
		if t.Status == "in_progress" {
			t.Status = "doing"
		}
		tasks = append(tasks, t)
	}

	return tasks, nil
}

func (s *TaskService) Update(ctx context.Context, workspaceID, taskID uuid.UUID, title, description, status string) (*models.Task, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	// Map frontend status to backend status
	backendStatus := status
	if status == "doing" {
		backendStatus = "in_progress"
	}

	var task models.Task
	err = s.db.QueryRow(ctx,
		`UPDATE tasks SET title = $1, description = $2, status = $3, updated_at = NOW() 
		 WHERE id = $4 AND workspace_id = $5 AND deleted_at IS NULL 
		 RETURNING id, workspace_id, project_id, title, description, status, deleted_at, created_at, updated_at`,
		title, description, backendStatus, taskID, workspaceID,
	).Scan(
		&task.ID, &task.WorkspaceID, &task.ProjectID, &task.Title,
		&task.Description, &task.Status, &task.DeletedAt,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, errors.New("task not found")
	}
	if err != nil {
		return nil, err
	}

	// Map backend status to frontend status
	if task.Status == "in_progress" {
		task.Status = "doing"
	}

	return &task, nil
}

func (s *TaskService) Delete(ctx context.Context, workspaceID, taskID uuid.UUID) error {
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
		`UPDATE tasks SET deleted_at = $1, updated_at = NOW() 
		 WHERE id = $2 AND workspace_id = $3 AND deleted_at IS NULL`,
		now, taskID, workspaceID,
	)
	return err
}
