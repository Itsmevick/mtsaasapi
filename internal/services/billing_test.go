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

type mockBillingConfig struct {
	stripeSecretKey     string
	stripeWebhookSecret string
}

func (m *mockBillingConfig) GetStripeSecretKey() string {
	return m.stripeSecretKey
}

func (m *mockBillingConfig) GetStripeWebhookSecret() string {
	return m.stripeWebhookSecret
}

// TestCreateSubscription_TestMode tests that CreateSubscription works in test mode
// (when STRIPE_SECRET_KEY is empty) and updates the database without making Stripe calls
func TestCreateSubscription_TestMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test database
	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create billing service with empty Stripe key (test mode)
	cfg := &mockBillingConfig{
		stripeSecretKey:     "", // Test mode
		stripeWebhookSecret: "",
	}
	billingService := services.NewBillingService(pool, cfg)

	// Create test data
	workspaceID := uuid.New()
	userID := uuid.New()

	// Create workspace with customer ID
	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug, plan, stripe_customer_id) VALUES ($1, 'Test Workspace', 'test-workspace', 'free', $2)`,
		workspaceID, "cus_test_12345",
	)
	require.NoError(t, err)

	// Create user
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

	// Create tenant context
	ctx := database.WithTenant(context.Background(), workspaceID)

	// Call CreateSubscription in test mode
	priceID := "price_test_123"
	err = billingService.CreateSubscription(ctx, workspaceID, priceID)
	require.NoError(t, err)

	// Verify database was updated correctly
	var subscriptionID string
	var plan string
	err = pool.QueryRow(context.Background(),
		`SELECT stripe_subscription_id, plan FROM workspaces WHERE id = $1`,
		workspaceID,
	).Scan(&subscriptionID, &plan)
	require.NoError(t, err)

	// Verify subscription ID was set (mock format)
	assert.NotEmpty(t, subscriptionID)
	assert.Contains(t, subscriptionID, "sub_test_")
	assert.Equal(t, "pro", plan, "Plan should be updated to 'pro'")

	// Verify the subscription ID format matches expected test mode format
	expectedPrefix := "sub_test_" + workspaceID.String()[:8]
	assert.Equal(t, expectedPrefix, subscriptionID, "Subscription ID should match test mode format")
}

// TestCreateSubscription_RequiresCustomer tests that CreateSubscription fails when customer is not found
func TestCreateSubscription_RequiresCustomer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup test database
	dbURL := "postgres://saas_user:saas_password@localhost:5432/saas_db_test?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create billing service with empty Stripe key (test mode)
	cfg := &mockBillingConfig{
		stripeSecretKey:     "", // Test mode
		stripeWebhookSecret: "",
	}
	billingService := services.NewBillingService(pool, cfg)

	// Create test data
	workspaceID := uuid.New()

	// Create workspace WITHOUT customer ID
	_, err = pool.Exec(context.Background(),
		`INSERT INTO workspaces (id, name, slug, plan) VALUES ($1, 'Test Workspace', 'test-workspace', 'free')`,
		workspaceID,
	)
	require.NoError(t, err)

	// Cleanup
	defer func() {
		pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	}()

	// Create tenant context
	ctx := database.WithTenant(context.Background(), workspaceID)

	// Call CreateSubscription - should fail because customer is missing
	priceID := "price_test_123"
	err = billingService.CreateSubscription(ctx, workspaceID, priceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "customer not found")
}

