package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

type BillingService struct {
	db              *pgxpool.Pool
	stripeSecretKey string
	webhookSecret   string
}

type BillingConfig interface {
	GetStripeSecretKey() string
	GetStripeWebhookSecret() string
}

// Config implements BillingConfig (defined in config package)
// This allows the config package to be used directly

func NewBillingService(db *pgxpool.Pool, cfg BillingConfig) *BillingService {
	return &BillingService{
		db:              db,
		stripeSecretKey: cfg.GetStripeSecretKey(),
		webhookSecret:   cfg.GetStripeWebhookSecret(),
	}
}

func (s *BillingService) CreateCustomer(ctx context.Context, workspaceID uuid.UUID, email string) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	if s.stripeSecretKey == "" {
		// Test mode - just set a mock customer ID
		mockCustomerID := "cus_test_" + workspaceID.String()[:8]
		_, err = s.db.Exec(ctx,
			`UPDATE workspaces SET stripe_customer_id = $1 WHERE id = $2`,
			mockCustomerID, workspaceID,
		)
		return err
	}

	stripe.Key = s.stripeSecretKey
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Metadata: map[string]string{
			"workspace_id": workspaceID.String(),
		},
	}

	cust, err := customer.New(params)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE workspaces SET stripe_customer_id = $1 WHERE id = $2`,
		cust.ID, workspaceID,
	)
	return err
}

func (s *BillingService) CreateSubscription(ctx context.Context, workspaceID uuid.UUID, priceID string) error {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return err
	}
	if tenantID != workspaceID {
		return errors.New("workspace ID mismatch")
	}

	var customerID string
	err = s.db.QueryRow(ctx,
		"SELECT stripe_customer_id FROM workspaces WHERE id = $1",
		workspaceID,
	).Scan(&customerID)
	if err != nil {
		return err
	}
	if customerID == "" {
		return errors.New("customer not found")
	}

	if s.stripeSecretKey == "" {
		// Test mode - just set a mock subscription ID
		mockSubID := "sub_test_" + workspaceID.String()[:8]
		_, err = s.db.Exec(ctx,
			`UPDATE workspaces SET stripe_subscription_id = $1, plan = 'pro' WHERE id = $2`,
			mockSubID, workspaceID,
		)
		return err
	}

	// Create subscription in Stripe
	stripe.Key = s.stripeSecretKey
	params := &stripe.SubscriptionParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(priceID),
			},
		},
		Metadata: map[string]string{
			"workspace_id": workspaceID.String(),
		},
	}

	sub, err := subscription.New(params)
	if err != nil {
		return err
	}

	// Determine plan from subscription (default to 'pro' for now)
	// In a production system, you might map priceID to plan names
	plan := "pro"

	// Update workspace with subscription ID and plan
	_, err = s.db.Exec(ctx,
		`UPDATE workspaces SET stripe_subscription_id = $1, plan = $2 WHERE id = $3`,
		sub.ID, plan, workspaceID,
	)
	return err
}

func (s *BillingService) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.webhookSecret == "" {
		return nil // Skip validation in test mode
	}

	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return err
	}

	switch event.Type {
	case "customer.subscription.updated", "customer.subscription.deleted":
		// Handle subscription updates
		var subscription stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
			return err
		}

		workspaceID := subscription.Metadata["workspace_id"]
		if workspaceID == "" {
			return errors.New("workspace_id not found in metadata")
		}

		workspaceUUID, err := uuid.Parse(workspaceID)
		if err != nil {
			return err
		}

		if event.Type == "customer.subscription.deleted" {
			_, err = s.db.Exec(ctx,
				`UPDATE workspaces SET stripe_subscription_id = NULL, plan = 'free' WHERE id = $1`,
				workspaceUUID,
			)
		} else {
			_, err = s.db.Exec(ctx,
				`UPDATE workspaces SET stripe_subscription_id = $1 WHERE id = $2`,
				subscription.ID, workspaceUUID,
			)
		}
		return err
	}

	return nil
}

func (s *BillingService) HasFeature(ctx context.Context, workspaceID uuid.UUID, feature string) (bool, error) {
	// Ensure tenant scoping
	tenantID, err := database.GetTenantID(ctx)
	if err != nil {
		return false, err
	}
	if tenantID != workspaceID {
		return false, errors.New("workspace ID mismatch")
	}

	var plan string
	err = s.db.QueryRow(ctx,
		"SELECT plan FROM workspaces WHERE id = $1",
		workspaceID,
	).Scan(&plan)
	if err != nil {
		return false, err
	}

	// Feature gating logic
	featureGates := map[string][]string{
		"advanced_analytics": {"pro", "enterprise"},
		"custom_integrations": {"enterprise"},
		"priority_support": {"pro", "enterprise"},
	}

	allowedPlans, exists := featureGates[feature]
	if !exists {
		return true, nil // Feature not gated
	}

	for _, allowedPlan := range allowedPlans {
		if plan == allowedPlan {
			return true, nil
		}
	}

	return false, nil
}