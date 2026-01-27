package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/render"
)

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Check database connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get database pool from auth service (which has access to db)
	db := h.AuthService.GetDB()
	if db == nil {
		render.Status(r, http.StatusServiceUnavailable)
		render.JSON(w, r, map[string]interface{}{
			"status":  "unhealthy",
			"service": "api",
			"error":   "database connection not initialized",
		})
		return
	}

	// Ping database
	if err := db.Ping(ctx); err != nil {
		render.Status(r, http.StatusServiceUnavailable)
		render.JSON(w, r, map[string]interface{}{
			"status":  "unhealthy",
			"service": "api",
			"error":   "database ping failed: " + err.Error(),
		})
		return
	}

	// All checks passed
	render.JSON(w, r, map[string]interface{}{
		"status":  "ok",
		"service": "api",
		"database": "connected",
	})
}
