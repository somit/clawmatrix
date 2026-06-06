package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"control-plane/internal/database"
)

// CreateMyToken mints a personal access token for the calling user. The raw token
// is returned exactly once.
func (h *Handlers) CreateMyToken(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	var req struct {
		Name      string `json:"name"`
		ExpiresIn int    `json:"expiresInDays"` // 0 = never
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().UTC().AddDate(0, 0, req.ExpiresIn)
		expiresAt = &t
	}
	raw, tok, err := database.CreateUserToken(u.ID, req.Name, expiresAt)
	if err != nil {
		respond(w, 500, J{"error": "failed to create token"})
		return
	}
	logAction("USER_TOKEN_CREATED", u.Username, u.Username)
	respond(w, 201, J{
		"id":        tok.ID,
		"name":      tok.Name,
		"token":     raw, // shown once
		"createdAt": tok.CreatedAt,
		"expiresAt": tok.ExpiresAt,
	})
}

// ListMyTokens lists the calling user's tokens (never the raw value).
func (h *Handlers) ListMyTokens(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	tokens, _ := database.ListUserTokens(u.ID)
	out := make([]J, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, J{
			"id":         t.ID,
			"name":       t.Name,
			"lastUsedAt": t.LastUsedAt,
			"expiresAt":  t.ExpiresAt,
			"createdAt":  t.CreatedAt,
		})
	}
	respond(w, 200, out)
}

// DeleteMyToken revokes one of the calling user's tokens.
func (h *Handlers) DeleteMyToken(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid token id"})
		return
	}
	if err := database.DeleteUserToken(u.ID, uint(id)); err != nil {
		respond(w, 404, J{"error": "token not found"})
		return
	}
	logAction("USER_TOKEN_REVOKED", u.Username, u.Username)
	respond(w, 200, J{"ok": true})
}

// MyAgents lists the agents the calling user is authorized to chat with, with the
// description that drives discovery-based delegation.
func (h *Handlers) MyAgents(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	profiles, _ := database.ListAgentProfiles()
	out := make([]J, 0, len(profiles))
	for _, p := range profiles {
		agents, _ := database.GetAgentsByName(p.Name)

		// A profile is listed if the user can chat it at the profile level, or
		// (agent-level grant) can chat at least one agent within it.
		allowed := database.UserHasProfilePerm(u.ID, p.Name, database.PermChatWithAgents)
		if !allowed {
			for _, a := range agents {
				if database.UserHasAgentPerm(u.ID, a.ID, database.PermChatWithAgents) {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			continue
		}

		status, runner := "offline", ""
		for _, a := range agents {
			if a.Status != "healthy" {
				continue
			}
			status = "healthy"
			var meta map[string]any
			json.Unmarshal([]byte(a.Meta), &meta)
			runner, _ = meta["runner"].(string)
			break
		}
		out = append(out, J{
			"name":        p.Name,
			"description": p.Description,
			"status":      status,
			"runner":      runner,
		})
	}
	respond(w, 200, out)
}
