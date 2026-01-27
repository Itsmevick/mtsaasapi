package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/config"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/handlers"
	"github.com/multitenant-saas/api/internal/middleware"
	"github.com/multitenant-saas/api/internal/router"
	"github.com/multitenant-saas/api/internal/services"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrossWorkspaceAccess tests that accessing resources from a different workspace returns 403/404
func TestCrossWorkspaceAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test database
	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create mock Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

	// Create mock config (using the one from auth_test.go)
	cfg := &mockConfig{
		sessionSecret: "test-secret",
	}

	// Setup services
	authService := services.NewAuthService(pool, redisClient, cfg)
	workspaceService := services.NewWorkspaceService(pool)
	memberService := services.NewMemberService(pool)
	projectService := services.NewProjectService(pool)
	taskService := services.NewTaskService(pool)
	activityService := services.NewActivityService(pool)
	billingService := services.NewBillingService(pool, cfg)
	dashboardService := services.NewDashboardService(pool)

	// Setup handlers
	// Create a minimal config for handlers and middleware
	testCfg := &config.Config{
		Env: "test",
	}
	h := handlers.New(
		authService,
		workspaceService,
		memberService,
		projectService,
		taskService,
		activityService,
		billingService,
		dashboardService,
		testCfg,
	)

	// Setup middleware
	mw := middleware.New(authService, testCfg, redisClient)

	// Setup router
	r := router.Setup(h, mw, testCfg)

	// Create test data: two users, two workspaces, one project in each
	user1ID := uuid.New()
	user2ID := uuid.New()
	workspace1ID := uuid.New()
	workspace2ID := uuid.New()
	member1ID := uuid.New()
	member2ID := uuid.New()

	// Create users
	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name) VALUES 
			($1, 'user1@example.com', 'hash1', 'User 1'),
			($2, 'user2@example.com', 'hash2', 'User 2')`,
		user1ID, user2ID,
	)
	require.NoError(t, err)

	// Create workspaces
	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug) VALUES 
			($1, 'Workspace 1', 'workspace-1'),
			($2, 'Workspace 2', 'workspace-2')`,
		workspace1ID, workspace2ID,
	)
	require.NoError(t, err)

	// Create memberships
	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspace_members (id, workspace_id, user_id, role) VALUES 
			($1, $3, $5, 'member'),
			($2, $4, $6, 'member')`,
		member1ID, member2ID, workspace1ID, workspace2ID, user1ID, user2ID,
	)
	require.NoError(t, err)

	// Create projects in each workspace
	project1ID := uuid.New()
	project2ID := uuid.New()

	ctx1 := database.WithTenant(context.Background(), workspace1ID)
	ctx2 := database.WithTenant(context.Background(), workspace2ID)

	_, err = pool.Exec(ctx1,
		`INSERT INTO projects (id, workspace_id, name, description) VALUES ($1, $2, 'Project 1', 'Description 1')`,
		project1ID, workspace1ID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx2,
		`INSERT INTO projects (id, workspace_id, name, description) VALUES ($1, $2, 'Project 2', 'Description 2')`,
		project2ID, workspace2ID,
	)
	require.NoError(t, err)

	// Cleanup
	defer func() {
		pool.Exec(context.Background(), `DELETE FROM projects WHERE id IN ($1, $2)`, project1ID, project2ID)
		pool.Exec(context.Background(), `DELETE FROM workspace_members WHERE id IN ($1, $2)`, member1ID, member2ID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, user1ID, user2ID)
		pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id IN ($1, $2)`, workspace1ID, workspace2ID)
	}()

	// Create sessions for both users
	session1, err := authService.CreateSession(context.Background(), user1ID)
	require.NoError(t, err)

	// Test 1: User 1 tries to access Project 2 (from Workspace 2) using Workspace 1 context
	// This should return 404 because the project doesn't exist in Workspace 1's scope
	req := httptest.NewRequest("GET", "/api/v1/projects/"+project2ID.String(), nil)
	req.Header.Set("X-Workspace-ID", workspace1ID.String())
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: session1,
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Should return 404 because project2 doesn't exist in workspace1's tenant scope
	assert.Equal(t, http.StatusNotFound, rr.Code, "Accessing another workspace's project should return 404")

	// Test 2: User 1 tries to access Project 2 using Workspace 2 context but without membership
	// This should return 403 because user1 is not a member of workspace2
	req2 := httptest.NewRequest("GET", "/api/v1/projects/"+project2ID.String(), nil)
	req2.Header.Set("X-Workspace-ID", workspace2ID.String())
	req2.AddCookie(&http.Cookie{
		Name:  "session",
		Value: session1,
	})

	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)

	// Should return 403 because user1 is not a member of workspace2
	assert.Equal(t, http.StatusForbidden, rr2.Code, "Accessing workspace without membership should return 403")
	assert.Contains(t, rr2.Body.String(), "Access denied")

	// Test 3: User 1 accesses their own project in Workspace 1 - should succeed
	req3 := httptest.NewRequest("GET", "/api/v1/projects/"+project1ID.String(), nil)
	req3.Header.Set("X-Workspace-ID", workspace1ID.String())
	req3.AddCookie(&http.Cookie{
		Name:  "session",
		Value: session1,
	})

	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)

	// Should return 200 because user1 is accessing their own project in their workspace
	assert.Equal(t, http.StatusOK, rr3.Code, "Accessing own workspace's project should succeed")
}

