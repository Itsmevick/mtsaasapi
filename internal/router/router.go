package router

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/multitenant-saas/api/internal/config"
	"github.com/multitenant-saas/api/internal/handlers"
	"github.com/multitenant-saas/api/internal/middleware"
)

func Setup(h *handlers.Handlers, mw *middleware.Middleware, cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.SecurityHeaders)
	r.Use(mw.CORS)
	r.Use(mw.RateLimit)
	r.Use(mw.Logging)
	r.Use(mw.Recoverer) // Custom recoverer with error logging

	// Health check (public)
	r.Get("/health", h.HealthCheck)

	// OpenAPI documentation
	SetupOpenAPI(r)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/register", h.Register)
		r.Post("/auth/login", h.Login)
		r.With(mw.RequireAuth).Post("/auth/logout", h.Logout)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireAuth)

			// User routes
			r.Get("/auth/me", h.Me)

			// Workspace routes
			r.Get("/workspaces", h.ListWorkspaces)
			r.Post("/workspaces", h.CreateWorkspace)
			r.Get("/workspaces/{id}", h.GetWorkspace)
			r.With(mw.RequireWorkspace, mw.RequireRole("admin")).Patch("/workspaces/{id}", h.UpdateWorkspace)

			// Workspace-scoped routes
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireWorkspace)

				// User info with workspace role
				r.Get("/me", h.MeWithWorkspace)

				// Projects
				r.Get("/projects", h.ListProjects)
				r.Post("/projects", h.CreateProject)
				r.Get("/projects/{id}", h.GetProject)
				r.Patch("/projects/{id}", h.UpdateProject)
				r.Delete("/projects/{id}", h.DeleteProject)

				// Tasks
				r.Get("/tasks", h.ListTasks)
				r.Post("/tasks", h.CreateTask)
				r.Get("/tasks/{id}", h.GetTask)
				r.Patch("/tasks/{id}", h.UpdateTask)
				r.Delete("/tasks/{id}", h.DeleteTask)

				// Members
				r.Get("/members", h.ListMembers)
				r.With(mw.RequireRole("admin")).Post("/members", h.AddMember)
				r.With(mw.RequireRole("admin")).Patch("/members/{id}", h.UpdateMemberRole)
				r.With(mw.RequireRole("admin")).Delete("/members/{id}", h.RemoveMember)

				// Activity
				r.Get("/activity", h.ListActivity)

				// Dashboard
				r.Get("/dashboard/summary", h.GetDashboardSummary)
				r.Get("/dashboard/tasks-timeseries", h.GetTasksTimeseries)

				// Billing (gated to admin/owner)
				r.Group(func(r chi.Router) {
					r.Use(mw.RequireRole("admin"))
					r.Get("/billing", h.GetBilling)
					r.Get("/billing/subscription", h.GetSubscription)
					r.Post("/billing/create-customer", h.CreateCustomer)
					r.Post("/billing/create-subscription", h.CreateSubscription)
				})

				// Billing feature checks (available to all members)
				r.Get("/billing/features/{feature}", h.CheckFeature)
			})
		})
	})

	return r
}
