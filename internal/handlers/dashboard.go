package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/render"
	"github.com/multitenant-saas/api/internal/database"
)

func (h *Handlers) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	summary, err := h.DashboardService.GetSummary(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, summary)
}

func (h *Handlers) GetTasksTimeseries(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	// Get days parameter, default to 30
	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		days, err = strconv.Atoi(daysStr)
		if err != nil || days <= 0 {
			days = 30
		}
	}

	timeseries, err := h.DashboardService.GetTasksTimeseries(r.Context(), workspaceID, days)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, timeseries)
}

