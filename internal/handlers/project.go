package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/multitenant-saas/api/internal/database"
)

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	project, err := h.ProjectService.Create(r.Context(), workspaceID, req.Name, req.Description)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "created", "project", project.ID, nil)

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, project)
}

func (h *Handlers) GetProject(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	projectIDStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid project ID"})
		return
	}

	project, err := h.ProjectService.GetByID(r.Context(), workspaceID, projectID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, project)
}

func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	projects, err := h.ProjectService.List(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, projects)
}

func (h *Handlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	projectIDStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid project ID"})
		return
	}

	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	project, err := h.ProjectService.Update(r.Context(), workspaceID, projectID, req.Name, req.Description)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "updated", "project", projectID, nil)

	render.JSON(w, r, project)
}

func (h *Handlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	projectIDStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid project ID"})
		return
	}

	if err := h.ProjectService.Delete(r.Context(), workspaceID, projectID); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "deleted", "project", projectID, nil)

	render.JSON(w, r, map[string]string{"message": "Project deleted"})
}
