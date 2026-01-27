package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/multitenant-saas/api/internal/database"
)

type CreateTaskRequest struct {
	ProjectID   *string `json:"projectId,omitempty"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}

type UpdateTaskRequest struct {
	ProjectID   *string `json:"projectId,omitempty"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
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

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	if req.Title == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Title is required"})
		return
	}

	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		parsed, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, map[string]string{"error": "Invalid project ID"})
			return
		}
		projectID = &parsed
	}

	status := "todo"
	if req.Status != nil && *req.Status != "" {
		status = *req.Status
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	task, err := h.TaskService.Create(r.Context(), workspaceID, projectID, req.Title, description, status)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "created", "task", task.ID, nil)

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, task)
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid task ID"})
		return
	}

	task, err := h.TaskService.GetByID(r.Context(), workspaceID, taskID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, task)
}

func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	var projectID *uuid.UUID
	projectIDStr := r.URL.Query().Get("projectId")
	if projectIDStr != "" {
		parsed, err := uuid.Parse(projectIDStr)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, map[string]string{"error": "Invalid project ID"})
			return
		}
		projectID = &parsed
	}

	status := r.URL.Query().Get("status")

	tasks, err := h.TaskService.List(r.Context(), workspaceID, projectID, status)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, tasks)
}

func (h *Handlers) UpdateTask(w http.ResponseWriter, r *http.Request) {
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

	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid task ID"})
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	// Get current task to preserve existing values
	currentTask, err := h.TaskService.GetByID(r.Context(), workspaceID, taskID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "Task not found"})
		return
	}

	// Use provided values or keep existing ones
	title := currentTask.Title
	if req.Title != nil {
		title = *req.Title
	}

	description := currentTask.Description
	if req.Description != nil {
		description = *req.Description
	}

	status := currentTask.Status
	if req.Status != nil {
		status = *req.Status
	}

	task, err := h.TaskService.Update(r.Context(), workspaceID, taskID, title, description, status)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "updated", "task", taskID, nil)

	render.JSON(w, r, task)
}

func (h *Handlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
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

	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid task ID"})
		return
	}

	if err := h.TaskService.Delete(r.Context(), workspaceID, taskID); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	h.ActivityService.Log(r.Context(), workspaceID, userID, "deleted", "task", taskID, nil)

	render.JSON(w, r, map[string]string{"message": "Task deleted"})
}
