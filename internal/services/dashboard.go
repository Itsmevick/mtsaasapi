package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
)

type DashboardService struct {
	db *pgxpool.Pool
}

func NewDashboardService(db *pgxpool.Pool) *DashboardService {
	return &DashboardService{db: db}
}

type DashboardSummary struct {
	ProjectsCount   int                    `json:"projectsCount"`
	TasksCount      int                    `json:"tasksCount"`
	MembersCount    int                    `json:"membersCount"`
	TasksByStatus   map[string]int          `json:"tasksByStatus"`
}

func (s *DashboardService) GetSummary(ctx context.Context, workspaceID uuid.UUID) (*DashboardSummary, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	summary := &DashboardSummary{
		TasksByStatus: make(map[string]int),
	}

	// Count total projects (non-deleted)
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE workspace_id = $1 AND deleted_at IS NULL`,
		workspaceID,
	).Scan(&summary.ProjectsCount)
	if err != nil {
		return nil, err
	}

	// Count total tasks (non-deleted)
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE workspace_id = $1 AND deleted_at IS NULL`,
		workspaceID,
	).Scan(&summary.TasksCount)
	if err != nil {
		return nil, err
	}

	// Count tasks by status
	rows, err := s.db.Query(ctx,
		`SELECT status, COUNT(*) 
		 FROM tasks 
		 WHERE workspace_id = $1 AND deleted_at IS NULL 
		 GROUP BY status`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		// Map "in_progress" from DB to "doing" for frontend
		if status == "in_progress" {
			summary.TasksByStatus["doing"] = count
		} else {
			summary.TasksByStatus[status] = count
		}
	}

	// Ensure all statuses are present (even if 0)
	if _, exists := summary.TasksByStatus["todo"]; !exists {
		summary.TasksByStatus["todo"] = 0
	}
	if _, exists := summary.TasksByStatus["doing"]; !exists {
		summary.TasksByStatus["doing"] = 0
	}
	if _, exists := summary.TasksByStatus["done"]; !exists {
		summary.TasksByStatus["done"] = 0
	}

	// Count active members
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $1`,
		workspaceID,
	).Scan(&summary.MembersCount)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

type TasksTimeseriesPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func (s *DashboardService) GetTasksTimeseries(ctx context.Context, workspaceID uuid.UUID, days int) ([]TasksTimeseriesPoint, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	// Validate days parameter
	if days <= 0 || days > 365 {
		days = 30 // Default to 30 days
	}

	// Calculate start date
	startDate := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	// Query tasks created per day
	rows, err := s.db.Query(ctx,
		`SELECT DATE(created_at) as date, COUNT(*) as count
		 FROM tasks
		 WHERE workspace_id = $1 
		   AND deleted_at IS NULL
		   AND created_at >= $2
		 GROUP BY DATE(created_at)
		 ORDER BY date ASC`,
		workspaceID, startDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TasksTimeseriesPoint
	dateMap := make(map[string]int)

	// Read results
	for rows.Next() {
		var date time.Time
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, err
		}
		dateStr := date.Format("2006-01-02")
		dateMap[dateStr] = count
	}

	// Fill in missing dates with 0
	currentDate := startDate
	endDate := time.Now().Truncate(24 * time.Hour)
	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format("2006-01-02")
		count := dateMap[dateStr]
		points = append(points, TasksTimeseriesPoint{
			Date:  dateStr,
			Count: count,
		})
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return points, nil
}

