package config

import "os"

type Config struct {
	JWTSecret       string
	DB              string
	DBURI           string
	Listen          string
	BootstrapConfig string
	// TLS (Let's Encrypt) — optional; only active when TLSDomain is set
	TLSDomain string // e.g. "cp.example.com"
	TLSEmail  string // optional, for LE expiry notifications
	// OIDC — optional; any compliant provider (Google, Okta, Keycloak, etc.)
	OIDCIssuerURL       string // e.g. "https://accounts.google.com"
	OIDCClientID        string
	OIDCClientSecret    string
	OIDCRedirectBaseURL string // e.g. "https://cp.example.com" (no trailing slash)
	OIDCProvider        string // google | github | microsoft | okta | keycloak | custom
}

func Load() *Config {
	db := envOr("DB", "sqlite")
	dbURI := os.Getenv("DB_URI")
	if dbURI == "" {
		// Backward compat: DB_PATH → DB_URI for sqlite
		if v := os.Getenv("DB_PATH"); v != "" {
			db = "sqlite"
			dbURI = v
		} else {
			dbURI = "/data/control-plane.db"
		}
	}

	return &Config{
		JWTSecret:           os.Getenv("JWT_SECRET"),
		DB:                  db,
		DBURI:               dbURI,
		Listen:              envOr("LISTEN", ":8080"),
		BootstrapConfig:     os.Getenv("BOOTSTRAP_CONFIG"),
		TLSDomain:           os.Getenv("TLS_DOMAIN"),
		TLSEmail:            os.Getenv("TLS_EMAIL"),
		OIDCIssuerURL:       os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientID:        os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:    os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCRedirectBaseURL: os.Getenv("OIDC_REDIRECT_BASE_URL"),
		OIDCProvider:        envOr("OIDC_PROVIDER", "custom"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
