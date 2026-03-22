package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"control-plane/internal/auth"
	"control-plane/internal/database"
)

// oidcProviderMeta holds display metadata for known providers.
var oidcProviderMeta = map[string]struct{ label, icon string }{
	"google":    {"Sign in with Google", "google"},
	"github":    {"Sign in with GitHub", "github"},
	"microsoft": {"Sign in with Microsoft", "microsoft"},
	"okta":      {"Sign in with Okta", "okta"},
	"keycloak":  {"Sign in with Keycloak", "keycloak"},
	"custom":    {"Sign in with SSO", ""},
}

// OIDCConfig holds provider settings. Nil means OIDC is disabled.
type OIDCConfig struct {
	Provider     string
	ButtonLabel  string
	ButtonIcon   string
	clientID     string
	clientSecret string
	redirectBase string
	provider     *gooidc.Provider
	oauth2Cfg    *oauth2.Config
}

// NewOIDCConfig initialises the OIDC provider via discovery. Returns nil if issuerURL is empty.
func NewOIDCConfig(issuerURL, clientID, clientSecret, redirectBase, providerName string) (*OIDCConfig, error) {
	if issuerURL == "" || clientID == "" {
		return nil, nil
	}
	oidcProvider, err := gooidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		return nil, err
	}
	meta, ok := oidcProviderMeta[providerName]
	if !ok {
		meta = oidcProviderMeta["custom"]
	}
	cfg := &OIDCConfig{
		Provider:     providerName,
		ButtonLabel:  meta.label,
		ButtonIcon:   meta.icon,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectBase: redirectBase,
		provider:     oidcProvider,
		oauth2Cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     oidcProvider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
		},
	}
	return cfg, nil
}

func (cfg *OIDCConfig) redirectURI(r *http.Request) string {
	if cfg.redirectBase != "" {
		return cfg.redirectBase + "/auth/oidc/callback"
	}
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/auth/oidc/callback"
}

// OIDCProviderConfig — GET /auth/oidc/config
// Returns whether OIDC is enabled and the button label, so the UI can show/hide the SSO button.
func (h *Handlers) OIDCProviderConfig(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		respond(w, 200, J{"enabled": false})
		return
	}
	respond(w, 200, J{
		"enabled":      true,
		"provider":     h.oidc.Provider,
		"button_label": h.oidc.ButtonLabel,
		"button_icon":  h.oidc.ButtonIcon,
	})
}

// OIDCStart — GET /auth/oidc/start
// Generates a state token, stores it in a short-lived cookie, redirects to the provider.
func (h *Handlers) OIDCStart(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		http.Error(w, "OIDC not configured", http.StatusNotFound)
		return
	}

	state, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/auth/oidc",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	o := *h.oidc.oauth2Cfg
	o.RedirectURL = h.oidc.redirectURI(r)
	http.Redirect(w, r, o.AuthCodeURL(state), http.StatusFound)
}

// OIDCCallback — GET /auth/oidc/callback
// Verifies state, exchanges code, looks up or links user, issues JWT.
func (h *Handlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		http.Error(w, "OIDC not configured", http.StatusNotFound)
		return
	}

	// Verify state
	cookie, err := r.Cookie("oidc_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/#oidc_error=invalid_state", http.StatusFound)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Path: "/auth/oidc", MaxAge: -1})

	o := *h.oidc.oauth2Cfg
	o.RedirectURL = h.oidc.redirectURI(r)

	ctx := r.Context()
	oauthToken, err := o.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/#oidc_error=exchange_failed", http.StatusFound)
		return
	}

	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		http.Redirect(w, r, "/#oidc_error=no_id_token", http.StatusFound)
		return
	}

	verifier := h.oidc.provider.Verifier(&gooidc.Config{ClientID: h.oidc.clientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Redirect(w, r, "/#oidc_error=invalid_token", http.StatusFound)
		return
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Sub == "" {
		http.Redirect(w, r, "/#oidc_error=missing_claims", http.StatusFound)
		return
	}

	// 1. Look up by external identity (returning user)
	u, err := database.GetUserByExternalIdentity("oidc", claims.Sub)
	if err != nil {
		http.Redirect(w, r, "/#oidc_error=db_error", http.StatusFound)
		return
	}

	// 2. No identity linked yet — try matching by email
	if u == nil && claims.Email != "" {
		u, err = database.GetUserByEmail(claims.Email)
		if err != nil {
			http.Redirect(w, r, "/#oidc_error=db_error", http.StatusFound)
			return
		}
		if u != nil {
			// Auto-link on first SSO login
			if err := database.LinkUserIdentity(u.ID, "oidc", claims.Sub); err != nil {
				http.Redirect(w, r, "/#oidc_error=link_failed", http.StatusFound)
				return
			}
		}
	}

	// 3. No user found — not registered
	if u == nil {
		http.Redirect(w, r, "/#oidc_error=not_registered", http.StatusFound)
		return
	}

	systemRole := ""
	if u.SystemRole != nil {
		systemRole = u.SystemRole.Name
	}
	jwt, err := auth.Sign(u.ID, u.Username, systemRole)
	if err != nil {
		http.Redirect(w, r, "/#oidc_error=sign_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/#oidc_token="+jwt, http.StatusFound)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
