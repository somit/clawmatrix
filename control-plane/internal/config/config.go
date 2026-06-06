package config

import (
	"os"
	"strings"
)

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
	// Attachments / uploads
	UploadBackend string // fs (default) | gcs | s3 | r2
	UploadDir     string // filesystem backend storage dir
	UploadBucket  string // bucket name for gcs/s3/r2 backends
	PublicBaseURL string // externally-reachable CP base URL for signed file links (no trailing slash)
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
		UploadBackend:       envOr("UPLOAD_BACKEND", "fs"),
		UploadDir:           envOr("UPLOAD_DIR", "/data/uploads"),
		UploadBucket:        os.Getenv("UPLOAD_BUCKET"),
		PublicBaseURL:       strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
