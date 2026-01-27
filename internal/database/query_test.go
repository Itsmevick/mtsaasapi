package database_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantScoping(t *testing.T) {
	// This test requires a running database
	// In CI, use testcontainers or a test database
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create two tenants
	tenant1ID := uuid.New()
	tenant2ID := uuid.New()

	ctx1 := database.WithTenant(context.Background(), tenant1ID)
	ctx2 := database.WithTenant(context.Background(), tenant2ID)

	// Create test data for tenant1
	_, err = pool.Exec(ctx1,
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Tenant 1', 'tenant1')`,
		tenant1ID,
	)
	require.NoError(t, err)

	// Create test data for tenant2
	_, err = pool.Exec(ctx2,
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Tenant 2', 'tenant2')`,
		tenant2ID,
	)
	require.NoError(t, err)

	// Test: tenant1 cannot access tenant2's data
	var count int
	err = pool.QueryRow(ctx1,
		`SELECT COUNT(*) FROM workspaces WHERE id = $1`,
		tenant2ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Tenant 1 should not see Tenant 2's workspace")

	// Test: tenant2 cannot access tenant1's data
	err = pool.QueryRow(ctx2,
		`SELECT COUNT(*) FROM workspaces WHERE id = $1`,
		tenant1ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Tenant 2 should not see Tenant 1's workspace")

	// Cleanup
	pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id IN ($1, $2)`, tenant1ID, tenant2ID)
}

func TestGetTenantID(t *testing.T) {
	tenantID := uuid.New()
	ctx := database.WithTenant(context.Background(), tenantID)

	retrievedID, err := database.GetTenantID(ctx)
	assert.NoError(t, err)
	assert.Equal(t, tenantID, retrievedID)

	// Test without tenant context
	_, err = database.GetTenantID(context.Background())
	assert.Error(t, err)
}
