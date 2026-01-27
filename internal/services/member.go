package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
)

type MemberService struct {
	db *pgxpool.Pool
}

func NewMemberService(db *pgxpool.Pool) *MemberService {
	return &MemberService{db: db}
}

func (s *MemberService) GetDB() *pgxpool.Pool {
	return s.db
}

// MemberWithUser represents a workspace member with user details
type MemberWithUser struct {
	UserID    uuid.UUID `json:"userId"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joinedAt"`
}

func (s *MemberService) List(ctx context.Context, workspaceID uuid.UUID) ([]MemberWithUser, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if tenantID != workspaceID {
		return nil, errors.New("workspace ID mismatch")
	}

	rows, err := s.db.Query(ctx,
		`SELECT wm.user_id, u.name, u.email, wm.role, wm.created_at
		 FROM workspace_members wm
		 INNER JOIN users u ON wm.user_id = u.id
		 WHERE wm.workspace_id = $1
		 ORDER BY wm.created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []MemberWithUser
	for rows.Next() {
		var m MemberWithUser
		err := rows.Scan(
			&m.UserID, &m.Name, &m.Email, &m.Role, &m.JoinedAt,
		)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}

	return members, nil
}

func (s *MemberService) UpdateRole(ctx context.Context, workspaceID, memberID uuid.UUID, role string) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	// Validate role
	validRoles := map[string]bool{"owner": true, "admin": true, "member": true}
	if !validRoles[role] {
		return errors.New("invalid role")
	}

	_, err = s.db.Exec(ctx,
		`UPDATE workspace_members SET role = $1, updated_at = NOW() 
		 WHERE id = $2 AND workspace_id = $3`,
		role, memberID, workspaceID,
	)
	return err
}

func (s *MemberService) Remove(ctx context.Context, workspaceID, memberID uuid.UUID) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	_, err = s.db.Exec(ctx,
		`DELETE FROM workspace_members 
		 WHERE id = $1 AND workspace_id = $2`,
		memberID, workspaceID,
	)
	return err
}

func (s *MemberService) Invite(ctx context.Context, workspaceID, inviterID uuid.UUID, email, role string) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	// Validate role
	validRoles := map[string]bool{"owner": true, "admin": true, "member": true}
	if !validRoles[role] {
		return errors.New("invalid role")
	}

	// Check if user exists
	var userID uuid.UUID
	err = s.db.QueryRow(ctx,
		"SELECT id FROM users WHERE email = $1",
		email,
	).Scan(&userID)

	if err == pgx.ErrNoRows {
		// User doesn't exist - return clear error message
		return errors.New("user not found, ask them to sign up first")
	}
	if err != nil {
		return err
	}

	// Check if user is already a member
	var existingMemberID uuid.UUID
	err = s.db.QueryRow(ctx,
		"SELECT id FROM workspace_members WHERE workspace_id = $1 AND user_id = $2",
		workspaceID, userID,
	).Scan(&existingMemberID)

	if err == nil {
		// User is already a member - return specific error for 409
		return errors.New("User already a member")
	}
	if err != pgx.ErrNoRows {
		return err
	}

	// Create membership
	_, err = s.db.Exec(ctx,
		`INSERT INTO workspace_members (workspace_id, user_id, role, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())`,
		workspaceID, userID, role,
	)
	return err
}
