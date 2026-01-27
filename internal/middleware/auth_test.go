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
	"github.com/multitenant-saas/api/internal/middleware"
	"github.com/multitenant-saas/api/internal/services"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestMiddleware creates a test middleware with a mock auth service
func setupTestMiddleware(t *testing.T) (*middleware.Middleware, *pgxpool.Pool) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Connect to test database
	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)

	// Create a mock Redis client (we won't use it for these tests)
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Create a mock config
	cfg := &mockConfig{
		sessionSecret: "test-secret",
	}

	// Create auth service
	authService := services.NewAuthService(pool, redisClient, cfg)

	// Create test config for middleware
	testCfg := &config.Config{
		Env:        "test",
		CORSOrigin: "http://localhost:3000",
	}

	// Create middleware
	mw := middleware.New(authService, testCfg, redisClient)

	return mw, pool
}

type mockConfig struct {
	sessionSecret string
}

func (m *mockConfig) GetSessionSecret() string {
	return m.sessionSecret
}

func (m *mockConfig) GetGitHubClientID() string {
	return ""
}

func (m *mockConfig) GetGitHubClientSecret() string {
	return ""
}

func (m *mockConfig) GetStripeSecretKey() string {
	return ""
}

func (m *mockConfig) GetStripeWebhookSecret() string {
	return ""
}

// TestRequireWorkspace_MissingHeader tests that requests without X-Workspace-ID return 400
func TestRequireWorkspace_MissingHeader(t *testing.T) {
	mw, pool := setupTestMiddleware(t)
	defer pool.Close()

	// Create a test handler
	handler := mw.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Create a request without X-Workspace-ID header
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	
	// Add user context (simulating RequireAuth middleware)
	userID := uuid.New()
	ctx := database.WithUser(req.Context(), userID)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Assert 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Workspace ID required")
}

// TestRequireWorkspace_InvalidUUID tests that requests with invalid workspace ID return 400
func TestRequireWorkspace_InvalidUUID(t *testing.T) {
	mw, pool := setupTestMiddleware(t)
	defer pool.Close()

	// Create a test handler
	handler := mw.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Create a request with invalid X-Workspace-ID header
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	req.Header.Set("X-Workspace-ID", "invalid-uuid")
	
	// Add user context
	userID := uuid.New()
	ctx := database.WithUser(req.Context(), userID)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Assert 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid workspace ID")
}

// TestRequireWorkspace_NotMember tests that requests from non-members return 403
func TestRequireWorkspace_NotMember(t *testing.T) {
	mw, pool := setupTestMiddleware(t)
	defer pool.Close()

	// Create test data
	userID := uuid.New()
	workspaceID := uuid.New()

	// Create workspace and user (but not as a member)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Test Workspace', 'test-workspace')`,
		workspaceID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, 'test@example.com', 'hash', 'Test User')`,
		userID,
	)
	require.NoError(t, err)

	// Cleanup
	defer func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	}()

	// Create a test handler
	handler := mw.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Create a request with valid X-Workspace-ID but user is not a member
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	req.Header.Set("X-Workspace-ID", workspaceID.String())
	
	// Add user context
	ctx := database.WithUser(req.Context(), userID)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Assert 403 Forbidden
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "Access denied")
}

// TestRequireWorkspace_ValidMember tests that requests from valid members succeed
func TestRequireWorkspace_ValidMember(t *testing.T) {
	mw, pool := setupTestMiddleware(t)
	defer pool.Close()

	// Create test data
	userID := uuid.New()
	workspaceID := uuid.New()
	memberID := uuid.New()

	// Create workspace, user, and membership
	_, err := pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Test Workspace', 'test-workspace')`,
		workspaceID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, 'test@example.com', 'hash', 'Test User')`,
		userID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspace_members (id, workspace_id, user_id, role) VALUES ($1, $2, $3, 'member')`,
		memberID, workspaceID, userID,
	)
	require.NoError(t, err)

	// Cleanup
	defer func() {
		pool.Exec(context.Background(), `DELETE FROM workspace_members WHERE id = $1`, memberID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	}()

	// Create a test handler
	handler := mw.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tenant context was set
		tenantID, err := database.GetTenantID(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, workspaceID, tenantID)
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Create a request with valid X-Workspace-ID and user is a member
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	req.Header.Set("X-Workspace-ID", workspaceID.String())
	
	// Add user context
	ctx := database.WithUser(req.Context(), userID)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Assert 200 OK
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "success")
}

// TestRequireWorkspace_QueryParam tests that workspace ID can be provided via query parameter
func TestRequireWorkspace_QueryParam(t *testing.T) {
	mw, pool := setupTestMiddleware(t)
	defer pool.Close()

	// Create test data
	userID := uuid.New()
	workspaceID := uuid.New()
	memberID := uuid.New()

	// Create workspace, user, and membership
	_, err := pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug) VALUES ($1, 'Test Workspace', 'test-workspace')`,
		workspaceID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, name) VALUES ($1, 'test@example.com', 'hash', 'Test User')`,
		userID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspace_members (id, workspace_id, user_id, role) VALUES ($1, $2, $3, 'member')`,
		memberID, workspaceID, userID,
	)
	require.NoError(t, err)

	// Cleanup
	defer func() {
		pool.Exec(context.Background(), `DELETE FROM workspace_members WHERE id = $1`, memberID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	}()

	// Create a test handler
	handler := mw.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tenant context was set
		tenantID, err := database.GetTenantID(r.Context())
		assert.NoError(t, err)
		assert.Equal(t, workspaceID, tenantID)
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	// Create a request with workspace ID in query parameter (no header)
	req := httptest.NewRequest("GET", "/api/v1/projects?workspace_id="+workspaceID.String(), nil)
	
	// Add user context
	ctx := database.WithUser(req.Context(), userID)
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Assert 200 OK
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "success")
}

