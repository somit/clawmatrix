package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"control-plane/internal/auth"
	"control-plane/internal/database"
)

type ctxKey string

const (
	ctxRegistration ctxKey = "registration"
	ctxUser         ctxKey = "user"
	ctxClaims       ctxKey = "claims"
)

// withAuth requires a valid JWT or personal access token (any authenticated user).
func (h *Handlers) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, claims, err := authUser(bearer(r))
		if err != nil {
			respond(w, 401, J{"error": "unauthorized"})
			return
		}
		ctx := context.WithValue(r.Context(), ctxClaims, claims)
		ctx = context.WithValue(ctx, ctxUser, u)
		next(w, r.WithContext(ctx))
	}
}

// authUser resolves a bearer token to a user. It accepts both a session JWT and a
// personal access token (PAT); the PAT path synthesizes claims so downstream code
// behaves identically. Returns an error if the token matches neither.
func authUser(tok string) (*database.User, *auth.Claims, error) {
	if claims, err := auth.Verify(tok); err == nil {
		if u, err := database.GetUserByID(claims.UserID); err == nil {
			return u, claims, nil
		}
	}
	if u, err := database.GetUserByToken(tok); err == nil && u != nil {
		role := ""
		if u.SystemRole != nil {
			role = u.SystemRole.Name
		}
		return u, &auth.Claims{UserID: u.ID, Username: u.Username, SystemRole: role}, nil
	}
	return nil, nil, errors.New("unauthorized")
}

// withPerm wraps withAuth and checks for a specific system permission.
func (h *Handlers) withPerm(perm string, next http.HandlerFunc) http.HandlerFunc {
	return h.withAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFromCtx(r)
		if !database.UserHasSystemPerm(u.ID, perm) {
			respond(w, 403, J{"error": "forbidden"})
			return
		}
		next(w, r)
	})
}

// withPermOrEmpty wraps withAuth for list (GET) endpoints: returns an empty JSON array
// instead of 403 when the user lacks the required system permission.
func (h *Handlers) withPermOrEmpty(perm string, next http.HandlerFunc) http.HandlerFunc {
	return h.withAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFromCtx(r)
		if !database.UserHasSystemPerm(u.ID, perm) {
			respond(w, 200, []any{})
			return
		}
		next(w, r)
	})
}

func (h *Handlers) withAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearer(r)
		reg, err := database.GetRegistrationByToken(tok)
		if err != nil {
			respond(w, 401, J{"error": "invalid token"})
			return
		}
		ctx := context.WithValue(r.Context(), ctxRegistration, reg)
		next(w, r.WithContext(ctx))
	}
}

// checkProfilePerm verifies the user has a profile-scoped permission; returns false and writes 403 if not.
func checkProfilePerm(w http.ResponseWriter, r *http.Request, profileName, perm string) bool {
	u := userFromCtx(r)
	if !database.UserHasProfilePerm(u.ID, profileName, perm) {
		respond(w, 403, J{"error": "forbidden"})
		return false
	}
	return true
}

// checkAgentPerm verifies the user has a permission on a specific agent, counting
// both agent-level grants and grants inherited from the agent's profile. Returns
// false and writes 403 if not.
func checkAgentPerm(w http.ResponseWriter, r *http.Request, agentID, perm string) bool {
	u := userFromCtx(r)
	if !database.UserHasAgentPerm(u.ID, agentID, perm) {
		respond(w, 403, J{"error": "forbidden"})
		return false
	}
	return true
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	return ""
}

func registrationFromCtx(r *http.Request) *database.Registration {
	return r.Context().Value(ctxRegistration).(*database.Registration)
}

func userFromCtx(r *http.Request) *database.User {
	return r.Context().Value(ctxUser).(*database.User)
}

func claimsFromCtx(r *http.Request) *auth.Claims {
	return r.Context().Value(ctxClaims).(*auth.Claims)
}
