package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"control-plane/internal/database"
)

// --- Human groups (teams) ---

func groupToJSON(g *database.HumanGroup, memberCount int64) J {
	return J{
		"id":          g.ID,
		"name":        g.Name,
		"description": g.Description,
		"memberCount": memberCount,
		"createdAt":   g.CreatedAt,
	}
}

func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := database.ListGroups()
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	out := make([]J, 0, len(groups))
	for i := range groups {
		members, _ := database.ListGroupMembers(groups[i].ID)
		out = append(out, groupToJSON(&groups[i], int64(len(members))))
	}
	respond(w, 200, out)
}

func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respond(w, 400, J{"error": "name required"})
		return
	}
	g, err := database.CreateGroup(req.Name, req.Description)
	if err != nil {
		respond(w, 400, J{"error": err.Error()})
		return
	}
	logAction("GROUP_CREATED", g.Name, g.Name)
	respond(w, 201, groupToJSON(g, 0))
}

func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid body"})
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) > 0 {
		if err := database.UpdateGroup(uint(id), updates); err != nil {
			respond(w, 500, J{"error": err.Error()})
			return
		}
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	if err := database.DeleteGroup(uint(id)); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	logAction("GROUP_DELETED", strconv.FormatUint(id, 10), "")
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	members, err := database.ListGroupMembers(uint(id))
	if err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	out := make([]J, 0, len(members))
	for _, m := range members {
		out = append(out, J{"id": m.ID, "username": m.Username, "email": m.Email})
	}
	respond(w, 200, out)
}

func (h *Handlers) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	var req struct {
		UserID uint `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 {
		respond(w, 400, J{"error": "user_id required"})
		return
	}
	if err := database.AddGroupMember(uint(id), req.UserID); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}

func (h *Handlers) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	userID, err := strconv.ParseUint(r.PathValue("user_id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid user_id"})
		return
	}
	if err := database.RemoveGroupMember(uint(id), uint(userID)); err != nil {
		respond(w, 500, J{"error": err.Error()})
		return
	}
	respond(w, 200, J{"ok": true})
}
