package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/services"
)

type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

type AddMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	members, err := h.MemberService.List(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, members)
}

func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	memberIDStr := chi.URLParam(r, "id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid member ID"})
		return
	}

	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid request"})
		return
	}

	if err := h.MemberService.UpdateRole(r.Context(), workspaceID, memberID, req.Role); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, map[string]string{"message": "Member role updated"})
}

func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := database.GetTenantID(r.Context())
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Workspace context required"})
		return
	}

	memberIDStr := chi.URLParam(r, "id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid member ID"})
		return
	}

	if err := h.MemberService.Remove(r.Context(), workspaceID, memberID); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	render.JSON(w, r, map[string]string{"message": "Member removed"})
}

func (h *Handlers) AddMember(w http.ResponseWriter, r *http.Request) {
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

	// Check if current user is owner or admin
	var currentUserRole string
	err = h.AuthService.GetDB().QueryRow(r.Context(),
		"SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2",
		workspaceID, userID,
	).Scan(&currentUserRole)

	if err != nil {
		render.Status(r, http.StatusForbidden)
		render.JSON(w, r, map[string]string{"error": "Access denied"})
		return
	}

	// Only owner and admin can add members
	if currentUserRole != "owner" && currentUserRole != "admin" {
		render.Status(r, http.StatusForbidden)
		render.JSON(w, r, map[string]string{"error": "Only admins and owners can add members"})
		return
	}

	var req AddMemberRequest
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

	// Default role to "member" if not provided
	role := req.Role
	if role == "" {
		role = "member"
	}

	// Validate role (only allow admin or member, not owner)
	validRoles := map[string]bool{"admin": true, "member": true}
	if !validRoles[role] {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": "Invalid role. Must be 'admin' or 'member'"})
		return
	}

	// Add the member
	err = h.MemberService.Invite(r.Context(), workspaceID, userID, req.Email, role)
	if err != nil {
		// Check for specific error types
		if err.Error() == "User already a member" {
			render.Status(r, http.StatusConflict)
			render.JSON(w, r, map[string]string{"error": "User already a member"})
			return
		}
		if err.Error() == "user not found, ask them to sign up first" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, map[string]string{"error": "User not found. Ask them to sign up first."})
			return
		}
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	// Get the newly added member to return
	members, err := h.MemberService.List(r.Context(), workspaceID)
	if err != nil {
		render.Status(r, http.StatusCreated)
		render.JSON(w, r, map[string]string{"message": "Member added successfully"})
		return
	}

	// Find the newly added member by email
	var newMember *services.MemberWithUser
	for i := range members {
		if members[i].Email == req.Email {
			newMember = &members[i]
			break
		}
	}

	if newMember != nil {
		render.Status(r, http.StatusCreated)
		render.JSON(w, r, newMember)
	} else {
		render.Status(r, http.StatusCreated)
		render.JSON(w, r, map[string]string{"message": "Member added successfully"})
	}
}
