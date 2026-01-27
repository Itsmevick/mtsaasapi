package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/multitenant-saas/api/internal/database"
)

func (h *Handlers) CheckFeature(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	feature := chi.URLParam(r, "feature")
	hasFeature, err := h.BillingService.HasFeature(r.Context(), workspaceID, feature)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, map[string]interface{}{
		"feature":    feature,
		"has_access": hasFeature,
	})
}

// GetBilling returns billing information for the workspace
func (h *Handlers) GetBilling(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	workspace, err := h.WorkspaceService.GetByID(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "Workspace not found"})
		return
	}

	render.JSON(w, r, map[string]interface{}{
		"plan":                 workspace.Plan,
		"stripe_customer_id":   workspace.StripeCustomerID,
		"stripe_subscription_id": workspace.StripeSubscriptionID,
	})
}

type CreateCustomerRequest struct {
	Email string `json:"email"`
}

// CreateCustomer creates a Stripe customer for the workspace
func (h *Handlers) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	var req CreateCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Email == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Email is required"})
		return
	}

	err = h.BillingService.CreateCustomer(r.Context(), workspaceID, req.Email)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Return updated workspace
	workspace, err := h.WorkspaceService.GetByID(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": "Failed to fetch workspace"})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"stripe_customer_id": workspace.StripeCustomerID,
		"message":            "Customer created successfully",
	})
}

type CreateSubscriptionRequest struct {
	PriceID string `json:"price_id"`
}

// CreateSubscription creates a Stripe subscription for the workspace
func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	if req.PriceID == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Price ID is required"})
		return
	}

	err = h.BillingService.CreateSubscription(r.Context(), workspaceID, req.PriceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Return updated workspace
	workspace, err := h.WorkspaceService.GetByID(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": "Failed to fetch workspace"})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"stripe_subscription_id": workspace.StripeSubscriptionID,
		"plan":                   workspace.Plan,
		"message":                "Subscription created successfully",
	})
}

// GetSubscription returns subscription details for the workspace
func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	workspace, err := h.WorkspaceService.GetByID(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "Workspace not found"})
		return
	}

	render.JSON(w, r, map[string]interface{}{
		"stripe_subscription_id": workspace.StripeSubscriptionID,
		"plan":                   workspace.Plan,
		"stripe_customer_id":     workspace.StripeCustomerID,
	})
}
