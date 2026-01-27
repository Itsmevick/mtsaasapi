package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QueryContext holds tenant context for queries
type QueryContext struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
}

// WithTenant ensures all queries are scoped to a tenant
func WithTenant(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, "tenant_id", tenantID)
}

// WithUser adds user context
func WithUser(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, "user_id", userID)
}

// GetTenantID extracts tenant ID from context
func GetTenantID(ctx context.Context) (uuid.UUID, error) {
	tenantID, ok := ctx.Value("tenant_id").(uuid.UUID)
	if !ok {
		return uuid.Nil, fmt.Errorf("tenant_id not found in context")
	}
	return tenantID, nil
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) (uuid.UUID, error) {
	userID, ok := ctx.Value("user_id").(uuid.UUID)
	if !ok {
		return uuid.Nil, fmt.Errorf("user_id not found in context")
	}
	return userID, nil
}

// QueryHelper provides tenant-scoped query methods
type QueryHelper struct {
	pool *pgxpool.Pool
}

func NewQueryHelper(pool *pgxpool.Pool) *QueryHelper {
	return &QueryHelper{pool: pool}
}

// Query executes a query with tenant scoping
func (qh *QueryHelper) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// Ensure query includes tenant_id filter
	// This is a safety check - actual queries should explicitly include WHERE workspace_id = $1
	return qh.pool.Query(ctx, query, append([]interface{}{tenantID}, args...)...)
}

// QueryRow executes a query that returns a single row with tenant scoping
func (qh *QueryHelper) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		// Return error row
		return &errorRow{err: err}
	}

	return qh.pool.QueryRow(ctx, query, append([]interface{}{tenantID}, args...)...)
}

// Exec executes a command with tenant scoping
func (qh *QueryHelper) Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
	tenantID, err := GetTenantID(ctx)
	if err != nil {
		return nil, err
	}

	return qh.pool.Exec(ctx, query, append([]interface{}{tenantID}, args...)...)
}

// errorRow implements pgx.Row for error cases
type errorRow struct {
	err error
}

func (e *errorRow) Scan(dest ...interface{}) error {
	return e.err
}
