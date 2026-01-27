package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/models"
)

type WorkspaceService struct {
	db *pgxpool.Pool
}

func NewWorkspaceService(db *pgxpool.Pool) *WorkspaceService {
	return &WorkspaceService{db: db}
}

func (s *WorkspaceService) Create(ctx context.Context, name string, ownerID uuid.UUID) (*models.Workspace, error) {
	// Generate slug from name
	slug := generateSlug(name)

	// Check if slug exists
	var existingID uuid.UUID
	err := s.db.QueryRow(ctx, "SELECT id FROM workspaces WHERE slug = $1", slug).Scan(&existingID)
	if err == nil {
		return nil, errors.New("workspace slug already exists")
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	// Create workspace
	var workspace models.Workspace
	err = s.db.QueryRow(ctx,
		`INSERT INTO workspaces (name, slug, plan) 
		 VALUES ($1, $2, 'free') 
		 RETURNING id, name, slug, plan, created_at, updated_at`,
		name, slug,
	).Scan(
		&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.Plan,
		&workspace.CreatedAt, &workspace.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Add owner as member
	_, err = s.db.Exec(ctx,
		`INSERT INTO workspace_members (workspace_id, user_id, role) 
		 VALUES ($1, $2, 'owner')`,
		workspace.ID, ownerID,
	)
	if err != nil {
		return nil, err
	}

	return &workspace, nil
}

func (s *WorkspaceService) GetByID(ctx context.Context, workspaceID uuid.UUID) (*models.Workspace, error) {
	var workspace models.Workspace
	err := s.db.QueryRow(ctx,
		`SELECT id, name, slug, plan, stripe_customer_id, stripe_subscription_id, created_at, updated_at 
		 FROM workspaces WHERE id = $1`,
		workspaceID,
	).Scan(
		&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.Plan,
		&workspace.StripeCustomerID, &workspace.StripeSubscriptionID,
		&workspace.CreatedAt, &workspace.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (s *WorkspaceService) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Workspace, error) {
	rows, err := s.db.Query(ctx,
		`SELECT w.id, w.name, w.slug, w.plan, w.stripe_customer_id, w.stripe_subscription_id, w.created_at, w.updated_at
		 FROM workspaces w
		 INNER JOIN workspace_members wm ON w.id = wm.workspace_id
		 WHERE wm.user_id = $1
		 ORDER BY w.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []models.Workspace
	for rows.Next() {
		var w models.Workspace
		err := rows.Scan(
			&w.ID, &w.Name, &w.Slug, &w.Plan,
			&w.StripeCustomerID, &w.StripeSubscriptionID,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}

	return workspaces, nil
}

func (s *WorkspaceService) Update(ctx context.Context, workspaceID uuid.UUID, name string) (*models.Workspace, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	var workspace models.Workspace
	err = s.db.QueryRow(ctx,
		`UPDATE workspaces SET name = $1, updated_at = NOW() 
		 WHERE id = $2 
		 RETURNING id, name, slug, plan, stripe_customer_id, stripe_subscription_id, created_at, updated_at`,
		name, workspaceID,
	).Scan(
		&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.Plan,
		&workspace.StripeCustomerID, &workspace.StripeSubscriptionID,
		&workspace.CreatedAt, &workspace.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	// Remove special characters
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
