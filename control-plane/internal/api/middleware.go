package api

import (
	"context"
	"net/http"
	"strings"

	"control-plane/internal/database"
)

type ctxKey string

const ctxRegistration ctxKey = "registration"

func (h *Handlers) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer(r) != h.adminToken {
			respond(w, 401, J{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
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
