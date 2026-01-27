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
		log.Error().Err(err).Str("user_id", user.ID.String()).Msg("Failed to create session during registration")
		render.Status(r, http.StatusInternalServerError)
		
		// Return detailed error in non-production, generic in production
		errorResponse := map[string]string{"error": "failed to create session"}
		if h.Config.Env != "production" {
			errorResponse["details"] = err.Error()
		}
		render.JSON(w, r, errorResponse)
		return
	}

	// Set cookie with production-safe settings
	isProduction := h.Config.Env == "production"
	cookie := &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400, // 24 hours
	}
	
	if isProduction {
		// Production: Secure=true, SameSite=None (required for cross-site), no Domain
		cookie.Secure = true
		cookie.SameSite = http.SameSiteNoneMode
		log.Info().Str("user_id", user.ID.String()).Msg("Setting production session cookie: Secure=true, SameSite=None, HttpOnly=true, Path=/")
	} else {
		// Development: Secure=false, SameSite=Lax
		cookie.Secure = false
		cookie.SameSite = http.SameSiteLaxMode
		log.Info().Str("user_id", user.ID.String()).Msg("Setting development session cookie: Secure=false, SameSite=Lax, HttpOnly=true, Path=/")
	}
	
	http.SetCookie(w, cookie)
	log.Info().Str("user_id", user.ID.String()).Msg("Session cookie set successfully")

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
		log.Error().Err(err).Str("user_id", user.ID.String()).Msg("Failed to create session during login")
		render.Status(r, http.StatusInternalServerError)
		
		// Return detailed error in non-production, generic in production
		errorResponse := map[string]string{"error": "failed to create session"}
		if h.Config.Env != "production" {
			errorResponse["details"] = err.Error()
		}
		render.JSON(w, r, errorResponse)
		return
	}

	// Set cookie with production-safe settings
	isProduction := h.Config.Env == "production"
	cookie := &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400, // 24 hours
	}
	
	if isProduction {
		// Production: Secure=true, SameSite=None (required for cross-site), no Domain
		cookie.Secure = true
		cookie.SameSite = http.SameSiteNoneMode
		log.Info().Str("user_id", user.ID.String()).Msg("Setting production session cookie: Secure=true, SameSite=None, HttpOnly=true, Path=/")
	} else {
		// Development: Secure=false, SameSite=Lax
		cookie.Secure = false
		cookie.SameSite = http.SameSiteLaxMode
		log.Info().Str("user_id", user.ID.String()).Msg("Setting development session cookie: Secure=false, SameSite=Lax, HttpOnly=true, Path=/")
	}
	
	http.SetCookie(w, cookie)
	log.Info().Str("user_id", user.ID.String()).Msg("Login success: session cookie set successfully")

	render.JSON(w, r, user)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		h.AuthService.DeleteSession(r.Context(), cookie.Value)
	}

	// Set cookie for logout with production-safe settings
	isProduction := h.Config.Env == "production"
	logoutCookie := &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	}
	
	if isProduction {
		logoutCookie.Secure = true
		logoutCookie.SameSite = http.SameSiteNoneMode
	} else {
		logoutCookie.Secure = false
		logoutCookie.SameSite = http.SameSiteLaxMode
	}
	
	http.SetCookie(w, logoutCookie)
	
	if isProduction {
		cookie.Secure = true
		cookie.SameSite = http.SameSiteNoneMode
	} else {
		cookie.Secure = false
		cookie.SameSite = http.SameSiteLaxMode
	}
	
	http.SetCookie(w, cookie)

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