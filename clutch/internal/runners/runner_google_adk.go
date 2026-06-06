package runners

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// googleAdkRunner handles Google ADK example agents through ADK's own CLI.
type googleAdkRunner struct {
	cfg Config
}

func (g *googleAdkRunner) CommandArgs(agent Agent, msg, session string) []string {
	args := splitFields(agent.Command)
	if len(args) == 0 {
		args = []string{"adk", "run"}
	}

	if agent.SessionsPath != "" {
		args = append(args, "--session_service_uri", agent.SessionsPath)
	}
	args = append(args, "--session_id", session, "--jsonl")

	agentDir := os.Getenv("ADK_AGENT_DIR")
	if agentDir == "" && os.Getenv("WORKSPACE_PATH") != "" {
		agentDir = filepath.Join(os.Getenv("WORKSPACE_PATH"), "adk")
	}
	if agentDir != "" {
		args = append(args, agentDir)
	}
	return append(args, msg)
}

func (g *googleAdkRunner) UsesStdin() bool              { return false }
func (g *googleAdkRunner) Env() []string                { return envAll() }
func (g *googleAdkRunner) DiscoverAgents() []Discovery  { return nil }
func (g *googleAdkRunner) SessionsPath(_ string) string { return g.cfg.SessionsPath }

func (g *googleAdkRunner) AgentCmd(_ string) string {
	if g.cfg.AgentCmd != "" {
		return g.cfg.AgentCmd
	}
	return "adk run"
}

func (g *googleAdkRunner) PrepareSession(agent Agent, _ string) {
	// ADK DB sessions are created by the adapter/native ADK runtime. Clutch only
	// prepares the local SQLite directory so the writer can create the DB file.
	if path := sqlitePath(agent.SessionsPath); path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0755)
	}
}

func (g *googleAdkRunner) ParseOutput(stdout, _ string) (string, string, map[string]any) {
	var texts []string
	var usage map[string]any

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var event struct {
			Author  string `json:"author"`
			Content struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			UsageMetadata map[string]any `json:"usageMetadata"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if len(event.UsageMetadata) > 0 {
			usage = event.UsageMetadata
		}
		if event.Author == "user" || event.Content.Role == "user" {
			continue
		}
		for _, part := range event.Content.Parts {
			if text := strings.TrimSpace(part.Text); text != "" {
				texts = append(texts, text)
			}
		}
	}

	return strings.Join(texts, "\n"), "", usage
}

func (g *googleAdkRunner) ServeSessions(w http.ResponseWriter, r *http.Request, agent Agent) bool {
	sessionsPath := agent.SessionsPath
	if sessionsPath == "" && agent.ID != "" {
		sessionsPath = g.SessionsPath(agent.ID)
	}
	if !isAdkDBSessionPath(sessionsPath) {
		return false
	}
	serveAdkDBSessions(w, r, sessionsPath)
	return true
}

func sqlitePath(dbURL string) string {
	if !strings.HasPrefix(dbURL, "sqlite:") && !strings.HasPrefix(dbURL, "sqlite+aiosqlite:") {
		return ""
	}
	path := strings.TrimPrefix(dbURL, "sqlite+aiosqlite:")
	path = strings.TrimPrefix(path, "sqlite:")
	if strings.HasPrefix(path, "////") {
		return path[3:]
	}
	if strings.HasPrefix(path, "///") {
		return path[3:]
	}
	if strings.HasPrefix(path, "//") {
		return strings.TrimPrefix(path, "//")
	}
	return path
}

func isAdkDBSessionPath(sessPath string) bool {
	return strings.HasPrefix(sessPath, "sqlite:") ||
		strings.HasPrefix(sessPath, "sqlite+aiosqlite:") ||
		strings.HasPrefix(sessPath, "postgres://") ||
		strings.HasPrefix(sessPath, "postgresql://")
}

func serveAdkDBSessions(w http.ResponseWriter, r *http.Request, dbURL string) {
	db, err := openAdkSessionDB(dbURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot open ADK sessions database: " + err.Error()})
		return
	}
	defer db.Close()

	name := r.URL.Query().Get("name")
	if name == "" {
		type sessionEntry struct {
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			Mtime string `json:"mtime"`
		}

		rows, err := db.Query("SELECT app_name, user_id, id, update_time FROM sessions ORDER BY update_time DESC")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot read ADK sessions: " + err.Error()})
			return
		}
		defer rows.Close()

		var out []sessionEntry
		for rows.Next() {
			var appName, userID, sessionID string
			var updateRaw any
			if err := rows.Scan(&appName, &userID, &sessionID, &updateRaw); err != nil {
				continue
			}
			out = append(out, sessionEntry{
				Name:  sessionID + ".json",
				Size:  0,
				Mtime: adkDBTime(updateRaw),
			})
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	appName, userID, sessionID, ok := resolveAdkSessionName(db, name)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ADK session name"})
		return
	}

	rows, err := db.Query(
		"SELECT event_data FROM events WHERE app_name = $1 AND user_id = $2 AND session_id = $3 ORDER BY timestamp ASC",
		appName,
		userID,
		sessionID,
	)
	if err != nil && strings.Contains(err.Error(), "near \"$1\"") {
		rows, err = db.Query(
			"SELECT event_data FROM events WHERE app_name = ? AND user_id = ? AND session_id = ? ORDER BY timestamp ASC",
			appName,
			userID,
			sessionID,
		)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot read ADK session events: " + err.Error()})
		return
	}
	defer rows.Close()

	type outMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type sessionOut struct {
		Key      string   `json:"key"`
		Messages []outMsg `json:"messages"`
	}

	var msgs []outMsg
	for rows.Next() {
		var eventJSON string
		if err := rows.Scan(&eventJSON); err != nil {
			continue
		}
		role, text := adkEventMessage(eventJSON)
		if text == "" {
			continue
		}
		msgs = append(msgs, outMsg{Role: role, Content: text})
	}

	writeJSON(w, http.StatusOK, sessionOut{Key: sessionID, Messages: msgs})
}

func openAdkSessionDB(dbURL string) (*sql.DB, error) {
	if strings.HasPrefix(dbURL, "sqlite:") || strings.HasPrefix(dbURL, "sqlite+aiosqlite:") {
		return sql.Open("sqlite", sqlitePath(dbURL))
	}
	return sql.Open("pgx", dbURL)
}

func resolveAdkSessionName(db *sql.DB, name string) (string, string, string, bool) {
	appName, userID, sessionID, ok := parseAdkSessionName(name)
	if ok {
		return appName, userID, sessionID, true
	}

	sessionID = strings.TrimSuffix(name, ".json")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		return "", "", "", false
	}

	row := db.QueryRow("SELECT app_name, user_id, id FROM sessions WHERE id = $1 ORDER BY update_time DESC LIMIT 1", sessionID)
	if err := row.Scan(&appName, &userID, &sessionID); err != nil {
		row = db.QueryRow("SELECT app_name, user_id, id FROM sessions WHERE id = ? ORDER BY update_time DESC LIMIT 1", sessionID)
		if err := row.Scan(&appName, &userID, &sessionID); err != nil {
			return "", "", "", false
		}
	}
	return appName, userID, sessionID, true
}

func parseAdkSessionName(name string) (string, string, string, bool) {
	name = strings.TrimSuffix(name, ".json")
	parts := strings.SplitN(name, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func adkDBTime(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case float64:
		return time.Unix(0, int64(t*1e9)).UTC().Format(time.RFC3339)
	case []byte:
		return adkDBTimeString(string(t))
	case string:
		return adkDBTimeString(t)
	default:
		return time.Now().UTC().Format(time.RFC3339)
	}
}

func adkDBTimeString(s string) string {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Unix(0, int64(f*1e9)).UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999999", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func adkEventMessage(eventJSON string) (string, string) {
	var event struct {
		Author  string `json:"author"`
		Content struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		return "", ""
	}

	var texts []string
	for _, part := range event.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	if len(texts) == 0 {
		return "", ""
	}

	role := "assistant"
	if event.Author == "user" || event.Content.Role == "user" {
		role = "user"
	}
	return role, strings.Join(texts, "\n")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
