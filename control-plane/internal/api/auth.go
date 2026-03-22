package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"control-plane/internal/auth"
	"control-plane/internal/database"
)

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		respond(w, 400, J{"error": "username and password required"})
		return
	}

	u, err := database.GetUserByUsername(req.Username)
	if err != nil || !database.CheckPassword(u, req.Password) {
		respond(w, 401, J{"error": "invalid credentials"})
		return
	}

	systemRole := ""
	if u.SystemRole != nil {
		systemRole = u.SystemRole.Name
	}

	token, err := auth.Sign(u.ID, u.Username, systemRole)
	if err != nil {
		respond(w, 500, J{"error": "could not sign token"})
		return
	}

	respond(w, 200, J{"token": token, "username": u.Username, "system_role": systemRole})
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	systemRole := ""
	var perms []string
	if u.SystemRole != nil {
		systemRole = u.SystemRole.Name
		for _, p := range u.SystemRole.Permissions {
			perms = append(perms, p.Permission)
		}
	}
	respond(w, 200, J{
		"id":          u.ID,
		"username":    u.Username,
		"email":       u.Email,
		"system_role": systemRole,
		"permissions": perms,
	})
}

// --- Users ---

func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := database.ListUsers()
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	type userResp struct {
		ID         uint    `json:"id"`
		Username   string  `json:"username"`
		Email      *string `json:"email,omitempty"`
		SystemRole string  `json:"system_role,omitempty"`
	}
	out := make([]userResp, len(users))
	for i, u := range users {
		role := ""
		if u.SystemRole != nil {
			role = u.SystemRole.Name
		}
		out[i] = userResp{ID: u.ID, Username: u.Username, Email: u.Email, SystemRole: role}
	}
	respond(w, 200, out)
}

func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username     string  `json:"username"`
		Password     string  `json:"password"`
		Email        *string `json:"email"`
		SystemRoleID *uint   `json:"system_role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		respond(w, 400, J{"error": "username and password required"})
		return
	}
	u, err := database.CreateUser(req.Username, req.Password, req.SystemRoleID, req.Email)
	if err != nil {
		respond(w, 409, J{"error": err.Error()})
		return
	}
	respond(w, 201, J{"id": u.ID, "username": u.Username})
}

func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	u, err := database.GetUserByID(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	role := ""
	if u.SystemRole != nil {
		role = u.SystemRole.Name
	}
	respond(w, 200, J{"id": u.ID, "username": u.Username, "email": u.Email, "system_role": role})
}

func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		Password     *string `json:"password"`
		Email        *string `json:"email"`
		SystemRoleID *uint   `json:"system_role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid body"})
		return
	}
	updates := map[string]any{}
	if req.SystemRoleID != nil {
		updates["system_role_id"] = req.SystemRoleID
	}
	if req.Email != nil {
		if *req.Email == "" {
			updates["email"] = nil
		} else {
			updates["email"] = req.Email
		}
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := hashPassword(*req.Password)
		if err != nil {
			respond(w, 500, J{"error": "could not hash password"})
			return
		}
		updates["password_hash"] = hash
	}
	if len(updates) == 0 {
		respond(w, 400, J{"error": "nothing to update"})
		return
	}
	if err := database.UpdateUser(uint(id), updates); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	// Prevent self-deletion
	caller := userFromCtx(r)
	if caller.ID == uint(id) {
		respond(w, 400, J{"error": "cannot delete yourself"})
		return
	}
	if err := database.DeleteUser(uint(id)); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

// --- User Identities ---

func (h *Handlers) ListUserIdentities(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	identities, err := database.ListUserIdentities(uint(id))
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	out := make([]J, len(identities))
	for i, ident := range identities {
		out[i] = J{"provider": ident.Provider, "external_id": ident.ExternalID}
	}
	respond(w, 200, out)
}

func (h *Handlers) LinkUserIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		Provider   string `json:"provider"`
		ExternalID string `json:"external_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider == "" || req.ExternalID == "" {
		respond(w, 400, J{"error": "provider and external_id required"})
		return
	}
	if err := database.LinkUserIdentity(uint(id), req.Provider, req.ExternalID); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) UnlinkUserIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	provider := r.PathValue("provider")
	if provider == "" {
		respond(w, 400, J{"error": "provider required"})
		return
	}
	if err := database.UnlinkUserIdentity(uint(id), provider); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

// --- Roles ---

func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := database.ListRoles()
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, roles)
}

func (h *Handlers) GetRole(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	role, err := database.GetRole(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	respond(w, 200, role)
}

func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || (req.Scope != "system" && req.Scope != "profile") {
		respond(w, 400, J{"error": "name and scope (system|profile) required"})
		return
	}
	role, err := database.CreateRole(req.Name, req.Description, req.Scope)
	if err != nil {
		respond(w, 409, J{"error": err.Error()})
		return
	}
	respond(w, 201, role)
}

func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid body"})
		return
	}
	if err := database.UpdateRole(uint(id), map[string]any{"description": req.Description}); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	if err := database.DeleteRole(uint(id)); err != nil {
		respond(w, 400, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) AddRolePermission(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Permission == "" {
		respond(w, 400, J{"error": "permission required"})
		return
	}
	if err := database.AddRolePermission(uint(id), req.Permission); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) RemoveRolePermission(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	perm := r.PathValue("perm")
	if err := database.RemoveRolePermission(uint(id), perm); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

// --- Profile ACL ---

func (h *Handlers) ListProfileACL(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("name")
	acls, err := database.ListProfileACL(profile)
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, acls)
}

func (h *Handlers) SetProfileACL(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("name")
	var req struct {
		UserID uint `json:"user_id"`
		RoleID uint `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 || req.RoleID == 0 {
		respond(w, 400, J{"error": "user_id and role_id required"})
		return
	}
	if err := database.SetProfileACL(profile, req.UserID, req.RoleID); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) DeleteProfileACL(w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("name")
	userID, err := strconv.ParseUint(r.PathValue("user_id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid user_id"})
		return
	}
	if err := database.DeleteProfileACL(profile, uint(userID)); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func hashPassword(password string) (string, error) {
	return database.HashPassword(password)
}
