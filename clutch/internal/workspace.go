package clutch

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxFileSize = 1 << 20 // 1MB

func handleWorkspaceForAgent(w http.ResponseWriter, r *http.Request, agent *RegisteredAgent) {
	if agent.workspace == "" {
		WriteJSON(w, 404, map[string]string{"error": "workspace not configured"})
		return
	}
	serveWorkspace(w, r, agent.workspace)
}

func handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if WorkspacePath == "" {
		WriteJSON(w, 404, map[string]string{"error": "workspace not configured"})
		return
	}
	serveWorkspace(w, r, WorkspacePath)
}

func serveWorkspace(w http.ResponseWriter, r *http.Request, wsPath string) {
	reqPath := r.URL.Query().Get("path")
	if reqPath == "" {
		reqPath = "."
	}

	clean := filepath.Clean(reqPath)
	full := filepath.Join(wsPath, clean)
	if !strings.HasPrefix(full, filepath.Clean(wsPath)) {
		WriteJSON(w, 403, map[string]string{"error": "path traversal denied"})
		return
	}

	info, err := os.Stat(full)
	if err != nil {
		WriteJSON(w, 404, map[string]string{"error": "not found"})
		return
	}

	if info.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			WriteJSON(w, 500, map[string]string{"error": "cannot read directory"})
			return
		}
		type fileEntry struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Size  int64  `json:"size"`
			Mtime string `json:"mtime"`
		}
		out := make([]fileEntry, 0, len(entries))
		for _, e := range entries {
			// Hide dotfiles except .claude (skills/commands live there)
			if strings.HasPrefix(e.Name(), ".") && e.Name() != ".claude" {
				continue
			}
			eInfo, err := e.Info()
			if err != nil {
				continue
			}
			t := "file"
			if e.IsDir() {
				t = "dir"
			}
			out = append(out, fileEntry{
				Name:  e.Name(),
				Type:  t,
				Size:  eInfo.Size(),
				Mtime: eInfo.ModTime().UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}

	if info.Size() > maxFileSize {
		WriteJSON(w, 413, map[string]string{"error": "file too large (>1MB)"})
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		WriteJSON(w, 500, map[string]string{"error": "cannot read file"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// --- Workspace Locks ---

const lockFileName = ".workspace-lock.json"

func readLockListFrom(wsPath string) []string {
	if wsPath == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(wsPath, lockFileName))
	if err != nil {
		return nil
	}
	var files []string
	json.Unmarshal(data, &files)
	return files
}

func writeLockListTo(wsPath string, files []string) error {
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(wsPath, lockFileName), data, 0644)
}

func handleWorkspaceLocksForAgent(w http.ResponseWriter, r *http.Request, agent *RegisteredAgent) {
	if agent.workspace == "" {
		WriteJSON(w, 404, map[string]string{"error": "workspace not configured"})
		return
	}
	serveWorkspaceLocks(w, r, agent.workspace)
}

func handleWorkspaceLocks(w http.ResponseWriter, r *http.Request) {
	if WorkspacePath == "" {
		WriteJSON(w, 404, map[string]string{"error": "workspace not configured"})
		return
	}
	serveWorkspaceLocks(w, r, WorkspacePath)
}

func serveWorkspaceLocks(w http.ResponseWriter, r *http.Request, wsPath string) {
	if r.Method == http.MethodGet {
		files := readLockListFrom(wsPath)
		if files == nil {
			files = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
		return
	}

	// PUT
	var req struct {
		Path   string `json:"path"`
		Locked bool   `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		WriteJSON(w, 400, map[string]string{"error": "path required"})
		return
	}

	clean := filepath.Clean(req.Path)
	full := filepath.Join(wsPath, clean)
	if !strings.HasPrefix(full, filepath.Clean(wsPath)) {
		WriteJSON(w, 403, map[string]string{"error": "path traversal denied"})
		return
	}

	if _, err := os.Stat(full); err != nil {
		WriteJSON(w, 404, map[string]string{"error": "file not found"})
		return
	}

	files := readLockListFrom(wsPath)
	if req.Locked {
		found := false
		for _, f := range files {
			if f == clean {
				found = true
				break
			}
		}
		if !found {
			files = append(files, clean)
		}
		if err := os.Chmod(full, 0444); err != nil {
			WriteJSON(w, 500, map[string]string{"error": "failed to lock: " + err.Error()})
			return
		}
		log.Printf("locked %s (chmod 444)", clean)
	} else {
		filtered := files[:0]
		for _, f := range files {
			if f != clean {
				filtered = append(filtered, f)
			}
		}
		files = filtered
		if err := os.Chmod(full, 0644); err != nil {
			WriteJSON(w, 500, map[string]string{"error": "failed to unlock: " + err.Error()})
			return
		}
		log.Printf("unlocked %s (chmod 644)", clean)
	}

	if err := writeLockListTo(wsPath, files); err != nil {
		WriteJSON(w, 500, map[string]string{"error": "failed to save lock list"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// --- Sessions Browser ---

func handleSessionsForAgent(w http.ResponseWriter, r *http.Request, agent *RegisteredAgent) {
	if agent.sessionsPath == "" {
		WriteJSON(w, 404, map[string]string{"error": "sessions not configured"})
		return
	}
	serveSessions(w, r, agent.sessionsPath)
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	if SessionsPath == "" {
		WriteJSON(w, 404, map[string]string{"error": "sessions not configured"})
		return
	}
	serveSessions(w, r, SessionsPath)
}

func serveSessions(w http.ResponseWriter, r *http.Request, sessPath string) {
	name := r.URL.Query().Get("name")

	if name == "" {
		entries, err := os.ReadDir(sessPath)
		if err != nil {
			WriteJSON(w, 500, map[string]string{"error": "cannot read sessions directory"})
			return
		}
		type sessionEntry struct {
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			Mtime string `json:"mtime"`
		}
		out := make([]sessionEntry, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, sessionEntry{
				Name:  e.Name(),
				Size:  info.Size(),
				Mtime: info.ModTime().UTC().Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}

	clean := filepath.Clean(name)
	full := filepath.Join(sessPath, clean)
	if !strings.HasPrefix(full, filepath.Clean(sessPath)) {
		WriteJSON(w, 403, map[string]string{"error": "path traversal denied"})
		return
	}

	data, err := os.ReadFile(full)
	if err != nil {
		WriteJSON(w, 404, map[string]string{"error": "session not found"})
		return
	}

	// JSONL files: delegate line parsing to the runner (each runner owns its format).
	if strings.Contains(clean, ".jsonl") {
		type outMsg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		type sessionOut struct {
			Messages []outMsg `json:"messages"`
		}

		runner := newRunner()
		var msgs []outMsg
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			role, content, ok := runner.ParseSessionLine(entry)
			if ok {
				msgs = append(msgs, outMsg{Role: role, Content: content})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessionOut{Messages: msgs})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
