package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/render"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/rs/zerolog/log"
)

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	user, err := h.AuthService.Register(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Create session
	sessionToken, err := h.AuthService.CreateSession(r.Context(), user.ID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": "Failed to create session"})
		return
	}

	// Set HttpOnly cookie with environment-aware settings
	isDevelopment := h.Config.Env == "development"
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   !isDevelopment, // true in production (https), false in development (http)
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	})

	render.JSON(w, r, user)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	user, err := h.AuthService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Create session
	sessionToken, err := h.AuthService.CreateSession(r.Context(), user.ID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": "Failed to create session"})
		return
	}

	// Set HttpOnly cookie for local development
	// Secure: false (required for http://localhost)
	// SameSite: Lax (works with http://localhost and allows credentials)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // false for local dev (http), true for production (https)
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	})

	// Dev log: login success
	log.Info().Str("user_id", user.ID.String()).Msg("login success: set session cookie")

	render.JSON(w, r, user)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		h.AuthService.DeleteSession(r.Context(), cookie.Value)
	}

	// Set HttpOnly cookie with environment-aware settings for logout
	isDevelopment := h.Config.Env == "development"
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !isDevelopment, // true in production (https), false in development (http)
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	render.JSON(w, r, map[string]string{"message": "Logged out"})
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	user, err := h.AuthService.GetUserByID(r.Context(), userID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "User not found"})
		return
	}

	render.JSON(w, r, user)
}

func (h *Handlers) MeWithWorkspace(w http.ResponseWriter, r *http.Request) {
	// Get workspace context
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	// Get user
	userID, err := database.GetUserID(r.Context())
	if err != nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]string{"error": "Unauthorized"})
		return
	}

	user, err := h.AuthService.GetUserByID(r.Context(), userID)
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, map[string]string{"error": "User not found"})
		return
	}

	// Get user's role in workspace
	var role string
	err = h.AuthService.GetDB().QueryRow(r.Context(),
		"SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2",
		workspaceID, userID,
	).Scan(&role)
	if err != nil {
		render.Status(r, http.StatusForbidden)
		render.JSON(w, r, map[string]string{"error": "No access to this workspace"})
		return
	}

	// Return user info with workspace role
	response := map[string]interface{}{
		"user": map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
		"workspaceRole": role,
	}

	render.JSON(w, r, response)
}