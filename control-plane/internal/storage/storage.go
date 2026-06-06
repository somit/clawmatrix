// Package storage abstracts attachment blob storage behind a small interface so
// the control plane can persist uploaded files to the local filesystem (default)
// or, once their SDKs are wired in, to GCS / S3 / Cloudflare R2 — without the
// rest of the code knowing which backend is active.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Store persists and retrieves attachment blobs by opaque id.
type Store interface {
	Put(id string, data []byte, mimeType string) error
	Get(id string) ([]byte, error)
	Delete(id string) error
	// Backend reports the backend name (fs | gcs | s3 | r2).
	Backend() string
}

// Presigner is implemented by object-store backends that can mint a time-limited
// URL the agent (or model) can fetch directly from the bucket. Filesystem does
// NOT implement this — the control plane serves those bytes itself instead.
type Presigner interface {
	// PresignGet returns a direct download URL valid for ttl. ok is false when
	// presigning is unavailable, signalling the caller to serve the bytes itself.
	PresignGet(id string, ttl time.Duration) (url string, ok bool)
}

// Config selects and parameterises a backend.
type Config struct {
	Backend string // fs | gcs | s3 | r2
	Dir     string // filesystem backend
	Bucket  string // object-store backends
}

// New builds a Store from cfg. Only "fs" is implemented today; the object-store
// backends are recognised but return a clear error until their SDKs are vendored.
func New(cfg Config) (Store, error) {
	switch cfg.Backend {
	case "", "fs":
		if cfg.Dir == "" {
			return nil, fmt.Errorf("storage: UPLOAD_DIR is required for the fs backend")
		}
		if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("storage: creating upload dir: %w", err)
		}
		return &fsStore{dir: cfg.Dir}, nil
	case "gcs", "s3", "r2":
		return nil, fmt.Errorf("storage: backend %q is not built into this binary yet (needs its cloud SDK); set UPLOAD_BACKEND=fs", cfg.Backend)
	default:
		return nil, fmt.Errorf("storage: unknown UPLOAD_BACKEND %q", cfg.Backend)
	}
}

// fsStore writes each blob as a single file under dir, named by id.
type fsStore struct {
	dir string
}

func (s *fsStore) Backend() string { return "fs" }

func (s *fsStore) path(id string) string { return filepath.Join(s.dir, filepath.Base(id)) }

func (s *fsStore) Put(id string, data []byte, _ string) error {
	return os.WriteFile(s.path(id), data, 0o644)
}

func (s *fsStore) Get(id string) ([]byte, error) {
	return os.ReadFile(s.path(id))
}

func (s *fsStore) Delete(id string) error {
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
