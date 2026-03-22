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

// withAdmin requires a valid JWT.
func (h *Handlers) withAdmin(next http.HandlerFunc) http.HandlerFunc {
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

// withPerm wraps withAdmin and checks for a specific system permission.
func (h *Handlers) withPerm(perm string, next http.HandlerFunc) http.HandlerFunc {
	return h.withAdmin(func(w http.ResponseWriter, r *http.Request) {
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
