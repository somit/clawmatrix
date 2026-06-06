package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"control-plane/internal/database"
	"control-plane/internal/storage"
)

// maxUploadBytes caps a single uploaded file. Uploads exist precisely so large
// files don't ride through the JSON-RPC call, but we still bound memory here.
const maxUploadBytes = 50 << 20 // 50 MiB

// uploadAccessTTL is how long a signed file-access link stays valid.
const uploadAccessTTL = 30 * time.Minute

// CreateUpload stores an attachment and returns its id + clawmatrix:// uri.
// Accepts either a multipart form ("file") or a raw body with X-Filename +
// Content-Type headers.
func (h *Handlers) CreateUpload(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respond(w, 501, J{"error": "uploads are not configured on this server"})
		return
	}
	u := userFromCtx(r)

	var (
		data     []byte
		name     string
		mimeType string
	)
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, hdr, err := r.FormFile("file")
		if err != nil {
			respond(w, 400, J{"error": "missing file field"})
			return
		}
		defer file.Close()
		if data, err = io.ReadAll(file); err != nil {
			respond(w, 400, J{"error": "file too large (max 50 MiB) or unreadable"})
			return
		}
		name = hdr.Filename
		mimeType = hdr.Header.Get("Content-Type")
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			respond(w, 400, J{"error": "file too large (max 50 MiB) or unreadable"})
			return
		}
		data = body
		name = r.Header.Get("X-Filename")
		mimeType = r.Header.Get("Content-Type")
	}
	if len(data) == 0 {
		respond(w, 400, J{"error": "empty file"})
		return
	}
	if name == "" {
		name = "attachment_" + randomID(6)
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = sniffMime(name, data)
	}

	id := "up_" + randomID(20)
	if err := h.store.Put(id, data, mimeType); err != nil {
		respond(w, 500, J{"error": "failed to store file"})
		return
	}
	up := &database.Upload{
		ID:       id,
		UserID:   u.ID,
		Name:     name,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Backend:  h.store.Backend(),
	}
	if err := database.CreateUpload(up); err != nil {
		h.store.Delete(id) // best-effort rollback
		respond(w, 500, J{"error": "failed to record upload"})
		return
	}
	logAction("UPLOAD_CREATED", u.Username, name)
	respond(w, 201, J{
		"id":       id,
		"uri":      "clawmatrix://uploads/" + id,
		"name":     name,
		"mimeType": mimeType,
		"size":     up.Size,
	})
}

// ListUploads lists the calling user's uploads (metadata only).
func (h *Handlers) ListUploads(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	ups, _ := database.ListUploads(u.ID)
	out := make([]J, 0, len(ups))
	for _, up := range ups {
		out = append(out, J{
			"id":        up.ID,
			"uri":       "clawmatrix://uploads/" + up.ID,
			"name":      up.Name,
			"mimeType":  up.MimeType,
			"size":      up.Size,
			"createdAt": up.CreatedAt,
			"expiresAt": up.ExpiresAt,
		})
	}
	respond(w, 200, out)
}

// DeleteUpload removes a blob + its row, scoped to the owner.
func (h *Handlers) DeleteUpload(w http.ResponseWriter, r *http.Request) {
	u := userFromCtx(r)
	id := r.PathValue("id")
	up, err := database.GetUpload(id)
	if err != nil || up.UserID != u.ID {
		respond(w, 404, J{"error": "upload not found"})
		return
	}
	if h.store != nil {
		h.store.Delete(id)
	}
	database.DeleteUpload(u.ID, id)
	respond(w, 200, J{"ok": true})
}

// GetUpload streams a stored file. Authorized by EITHER a valid signed query
// token (?token=…, used by agents fetching an attachment link) OR an owner
// bearer token. For object-store backends it 302-redirects to a presigned URL.
func (h *Handlers) GetUpload(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respond(w, 501, J{"error": "uploads are not configured on this server"})
		return
	}
	id := r.PathValue("id")
	up, err := database.GetUpload(id)
	if err != nil {
		respond(w, 404, J{"error": "upload not found"})
		return
	}

	authorized := false
	if tok := r.URL.Query().Get("token"); tok != "" {
		authorized = h.verifyUploadToken(id, tok)
	}
	if !authorized {
		if user, _, err := authUser(bearer(r)); err == nil && user != nil {
			authorized = user.ID == up.UserID || isAdmin(user)
		}
	}
	if !authorized {
		respond(w, 401, J{"error": "unauthorized"})
		return
	}

	// Object-store backends can hand the client a direct presigned URL.
	if p, ok := h.store.(storage.Presigner); ok {
		if url, ok := p.PresignGet(id, uploadAccessTTL); ok {
			http.Redirect(w, r, url, http.StatusFound)
			return
		}
	}

	data, err := h.store.Get(id)
	if err != nil {
		respond(w, 404, J{"error": "file bytes not found"})
		return
	}
	w.Header().Set("Content-Type", up.MimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", "inline; filename=\""+strings.ReplaceAll(up.Name, "\"", "")+"\"")
	w.WriteHeader(200)
	w.Write(data)
}

// --- signed access links ---

// uploadAccessURL builds an absolute, time-limited link an agent can fetch
// without a bearer token. Requires PublicBaseURL to be set for absolute links.
func (h *Handlers) uploadAccessURL(id string) string {
	exp := time.Now().Add(uploadAccessTTL).Unix()
	tok := fmt.Sprintf("%d.%s", exp, h.signUpload(id, exp))
	path := "/uploads/" + id + "?token=" + tok
	if h.publicBaseURL != "" {
		return h.publicBaseURL + path
	}
	return path
}

func (h *Handlers) signUpload(id string, exp int64) string {
	mac := hmac.New(sha256.New, h.signSecret)
	fmt.Fprintf(mac, "%s.%d", id, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handlers) verifyUploadToken(id, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	want := h.signUpload(id, exp)
	return hmac.Equal([]byte(want), []byte(parts[1]))
}

func isAdmin(u *database.User) bool {
	return u != nil && u.SystemRole != nil && u.SystemRole.Name == "admin"
}

// uploadRef is a single attachment reference (a clawmatrix://uploads/<id> uri,
// optionally with a display name overriding the stored filename).
type uploadRef struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// attachmentBlock resolves upload references into a text block carrying a signed,
// time-limited download link per file, suitable for injecting into a prompt.
// Non-admin callers may only reference uploads they own; pass admin=true to skip
// the ownership check (e.g. agent-to-agent senders).
func (h *Handlers) attachmentBlock(userID uint, admin bool, refs []uploadRef) (string, error) {
	var lines []string
	for _, ref := range refs {
		if ref.URI == "" {
			return "", fmt.Errorf("attachment is missing a uri")
		}
		if !strings.HasPrefix(ref.URI, uploadURIPrefix) {
			return "", fmt.Errorf("unsupported attachment uri %q (expected %s<id>)", ref.URI, uploadURIPrefix)
		}
		id := strings.TrimPrefix(ref.URI, uploadURIPrefix)
		up, err := database.GetUpload(id)
		if err != nil {
			return "", fmt.Errorf("attachment %s not found", id)
		}
		if !admin && up.UserID != userID {
			return "", fmt.Errorf("not authorized to attach upload %s", id)
		}
		name := up.Name
		if ref.Name != "" {
			name = ref.Name
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, %s): %s", name, up.MimeType, humanSize(up.Size), h.uploadAccessURL(id)))
	}
	if len(lines) == 0 {
		return "", nil
	}
	return "[Attachments]\n" + strings.Join(lines, "\n") + "\n\nFetch each attachment from its link to read its contents.", nil
}

// sniffMime picks a content type from the file extension, falling back to
// net/http content sniffing on the bytes.
func sniffMime(name string, data []byte) string {
	switch {
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".gif"):
		return "image/gif"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(name, ".txt"), strings.HasSuffix(name, ".log"), strings.HasSuffix(name, ".md"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	case strings.HasSuffix(name, ".csv"):
		return "text/csv"
	}
	return http.DetectContentType(data)
}
