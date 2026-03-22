package api

import (
	"context"
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

// withAuth requires a valid JWT (any authenticated user).
func (h *Handlers) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := auth.Verify(bearer(r))
		if err != nil {
			respond(w, 401, J{"error": "unauthorized"})
			return
		}
		u, err := database.GetUserByID(claims.UserID)
		if err != nil {
			respond(w, 401, J{"error": "unauthorized"})
			return
		}
		ctx := context.WithValue(r.Context(), ctxClaims, claims)
		ctx = context.WithValue(ctx, ctxUser, u)
		next(w, r.WithContext(ctx))
	}
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
