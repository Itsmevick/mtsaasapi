package services_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectService_TenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	service := services.NewProjectService(pool)

	// Create two tenants
	tenant1ID := uuid.New()
	tenant2ID := uuid.New()

	// Create workspaces
	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Tenant 1', 'tenant1'), ($2, 'Tenant 2', 'tenant2')`,
		tenant1ID, tenant2ID,
	)
	require.NoError(t, err)

	ctx1 := database.WithTenant(context.Background(), tenant1ID)
	ctx2 := database.WithTenant(context.Background(), tenant2ID)

	// Create project for tenant1
	project1, err := service.Create(ctx1, tenant1ID, "Project 1", "Description 1")
	require.NoError(t, err)

	// Create project for tenant2
	project2, err := service.Create(ctx2, tenant2ID, "Project 2", "Description 2")
	require.NoError(t, err)

	// Test: tenant1 cannot access tenant2's project
	_, err = service.GetByID(ctx1, tenant1ID, project2.ID)
	assert.Error(t, err, "Tenant 1 should not be able to access Tenant 2's project")

	// Test: tenant2 cannot access tenant1's project
	_, err = service.GetByID(ctx2, tenant2ID, project1.ID)
	assert.Error(t, err, "Tenant 2 should not be able to access Tenant 1's project")

	// Test: tenant1 can access their own project
	p1, err := service.GetByID(ctx1, tenant1ID, project1.ID)
	assert.NoError(t, err)
	assert.Equal(t, project1.ID, p1.ID)

	// Test: tenant2 can access their own project
	p2, err := service.GetByID(ctx2, tenant2ID, project2.ID)
	assert.NoError(t, err)
	assert.Equal(t, project2.ID, p2.ID)

	// Cleanup
	pool.Exec(context.Background(), `DELETE FROM projects WHERE id IN ($1, $2)`, project1.ID, project2.ID)
	pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id IN ($1, $2)`, tenant1ID, tenant2ID)
}
