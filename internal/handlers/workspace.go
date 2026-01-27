package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/rs/zerolog/log"
)

type CreateWorkspaceRequest struct {
	Name string `json:"name"`
}

func (h *Handlers) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	// Validate and trim name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace name is required"})
		return
	}

	workspace, err := h.WorkspaceService.Create(r.Context(), name, userID)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Log activity
	h.ActivityService.Log(r.Context(), workspace.ID, userID, "created", "workspace", workspace.ID, nil)

	// Dev log: workspace created
	log.Info().
		Str("workspace_id", workspace.ID.String()).
		Str("workspace_name", workspace.Name).
		Str("user_id", userID.String()).
		Msg("workspace created: workspace and membership created")

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, workspace)
}

func (h *Handlers) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := chi.URLParam(r, "id")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid workspace ID"})
		return
	}

	workspace, err := h.WorkspaceService.GetByID(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "Workspace not found"})
		return
	}

	render.JSON(w, r, workspace)
}

func (h *Handlers) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	workspaces, err := h.WorkspaceService.ListByUser(r.Context(), userID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, workspaces)
}

func (h *Handlers) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := chi.URLParam(r, "id")
	workspaceID, err := uuid.Parse(workspaceIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid workspace ID"})
		return
	}

	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	workspace, err := h.WorkspaceService.Update(r.Context(), workspaceID, req.Name)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	userID, _ := database.GetUserID(r.Context())
	h.ActivityService.Log(r.Context(), workspaceID, userID, "updated", "workspace", workspaceID, nil)

	render.JSON(w, r, workspace)
}
