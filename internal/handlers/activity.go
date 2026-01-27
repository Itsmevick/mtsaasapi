package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/render"
	"github.com/multitenant-saas/api/internal/database"
)

func (h *Handlers) ListActivity(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	activities, err := h.ActivityService.List(r.Context(), workspaceID, limit)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, activities)
}
