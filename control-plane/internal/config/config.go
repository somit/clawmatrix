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
		JWTSecret:       os.Getenv("JWT_SECRET"),
		DB:              db,
		DBURI:           dbURI,
		Listen:          envOr("LISTEN", ":8080"),
		BootstrapConfig: os.Getenv("BOOTSTRAP_CONFIG"),
		TLSDomain:       os.Getenv("TLS_DOMAIN"),
		TLSEmail:        os.Getenv("TLS_EMAIL"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
