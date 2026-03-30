package clutch

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// claudeCodeSessions maps agentID → clutchSession → claudeSessionID.
// Owned entirely by the claude-code runner; no other package touches it.
var (
	claudeCodeSessions   = map[string]map[string]string{}
	claudeCodeSessionsMu sync.Mutex
)

// claudeCodeRunner handles Claude Code CLI subprocess execution.
// Each /ask spawns `claude -p <msg> --output-format json [--resume <session-id>]`
// with the agent's workspace as the working directory.
type claudeCodeRunner struct{}

func (c *claudeCodeRunner) CommandArgs(agent *RegisteredAgent, msg, session string) []string {
	args := []string{"claude", "-p", msg, "--output-format", "json", "--verbose", "--bare", "--dangerously-skip-permissions"}
	if model := os.Getenv("CLAUDE_MODEL"); model != "" {
		args = append(args, "--model", model)
	}
	// For multi-agent mode where each agent has a distinct workspace,
	// pass --add-dir so Claude sees the right project folder regardless
	// of clutch's own working directory.
	if agent.workspace != "" {
		args = append(args, "--add-dir", agent.workspace)
	}
	// Resume an existing session if we have a mapping from a prior request.
	claudeCodeSessionsMu.Lock()
	claudeID := claudeCodeSessions[agent.id][session]
	claudeCodeSessionsMu.Unlock()
	if claudeID != "" {
		args = append(args, "--resume", claudeID)
		log.Printf("resuming session: clutch=%q claude=%q", session, claudeID)
	}
	return args
}

func (c *claudeCodeRunner) StoreSession(agentID, clutchSession, claudeSession string) {
	claudeCodeSessionsMu.Lock()
	defer claudeCodeSessionsMu.Unlock()
	if claudeCodeSessions[agentID] == nil {
		claudeCodeSessions[agentID] = map[string]string{}
	}
	claudeCodeSessions[agentID][clutchSession] = claudeSession
	log.Printf("session stored: clutch=%q claude=%q", clutchSession, claudeSession)
}

func (c *claudeCodeRunner) NormalizeSession(agentID, session string) string {
	claudeCodeSessionsMu.Lock()
	defer claudeCodeSessionsMu.Unlock()
	sessions := claudeCodeSessions[agentID]
	if sessions == nil {
		return session
	}
	if _, ok := sessions[session]; ok {
		return session
	}
	bare := strings.TrimSuffix(session, ".jsonl")
	for clutchKey, claudeID := range sessions {
		if claudeID == bare || claudeID+".jsonl" == session {
			log.Printf("session normalised: %q → %q (claude=%q)", session, clutchKey, claudeID)
			return clutchKey
		}
	}
	return session
}

func (c *claudeCodeRunner) UsesStdin() bool { return false }

func (c *claudeCodeRunner) Env() []string { return envAll() }

func (c *claudeCodeRunner) PrepareSession(_ *RegisteredAgent, _ string) {}

func (c *claudeCodeRunner) ParseOutput(stdout, _ string) (string, string, map[string]any) {
	return parseClaudeCodeOutput(stdout)
}

func (c *claudeCodeRunner) AgentCmd(_ string) string { return "claude" }

func (c *claudeCodeRunner) ParseSessionLine(entry map[string]any) (string, string, bool) {
	entryType, _ := entry["type"].(string)
	if entryType != "user" && entryType != "assistant" {
		return "", "", false
	}
	msg, _ := entry["message"].(map[string]any)
	if msg == nil {
		return "", "", false
	}
	role := entryType
	var content string
	switch c := msg["content"].(type) {
	case string:
		content = c
	case []any:
		var parts []string
		for _, b := range c {
			if block, ok := b.(map[string]any); ok && block["type"] == "text" {
				if t, _ := block["text"].(string); strings.TrimSpace(t) != "" {
					parts = append(parts, t)
				}
			}
		}
		content = strings.Join(parts, "\n")
	}
	if strings.TrimSpace(content) == "" {
		return "", "", false
	}
	return role, content, true
}

func (c *claudeCodeRunner) SessionsPath(localID string) string {
	return claudeCodeSessionsPath(localID)
}

func (c *claudeCodeRunner) DiscoverAgents() []agentDiscovery {
	return discoverClaudeCodeAgents()
}

// writeClaudeAllowlist writes the egress allowlist from the control plane to
// {workspace}/.claude/allowlist.json so the PreToolUse hook can read it.
// Only written when Runner == "claude-code" and WorkspacePath is set.
func writeClaudeAllowlist(domains []string) {
	if Runner != "claude-code" {
		return
	}
	ws := WorkspacePath
	if ws == "" {
		return
	}
	dir := filepath.Join(ws, ".claude")
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("allowlist: mkdir %s: %v", dir, err)
		return
	}
	data, _ := json.MarshalIndent(map[string]any{"domains": domains}, "", "  ")
	path := filepath.Join(dir, "allowlist.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("allowlist: write %s: %v", path, err)
		return
	}
	log.Printf("allowlist: wrote %d domains to %s", len(domains), path)
}

// claudeCodeResult is the JSON structure produced by `claude --output-format json`.
type claudeCodeResult struct {
	Type      string         `json:"type"`
	Subtype   string         `json:"subtype"`
	IsError   bool           `json:"is_error"`
	Result    string         `json:"result"`
	SessionID string         `json:"session_id"`
	CostUSD   float64        `json:"total_cost_usd"`
	Usage     map[string]any `json:"usage"`
}

func parseClaudeCodeOutput(raw string) (string, string, map[string]any) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	var out claudeCodeResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// Not JSON — return raw as-is.
		return raw, "", nil
	}
	if out.IsError {
		return "", "", nil
	}
	usage := out.Usage
	if out.SessionID != "" {
		if usage == nil {
			usage = map[string]any{}
		}
		usage["session_id"] = out.SessionID
	}
	if out.CostUSD > 0 {
		if usage == nil {
			usage = map[string]any{}
		}
		usage["cost_usd"] = out.CostUSD
	}
	return out.Result, "", usage
}

// claudeCodeSessionsPath returns the Claude Code sessions directory for a given agent.
// Claude stores sessions at ~/.claude/projects/<encoded-path>/ where the directory
// name is the workspace path with slashes replaced by hyphens.
// Falls back to ~/.claude/projects/ if no workspace is configured.
func claudeCodeSessionsPath(localID string) string {
	home, _ := os.UserHomeDir()
	base := home + "/.claude/projects"

	// Find the workspace for this agent from the config.
	agents := discoverClaudeCodeAgents()
	for _, a := range agents {
		if a.ID == localID && a.Workspace != "" {
			// Claude encodes the project path by replacing every '/' with '-'.
			// e.g. /Users/somit/myapp → -Users-somit-myapp
			return base + "/" + strings.ReplaceAll(a.Workspace, "/", "-")
		}
	}
	// Single-agent mode: derive from WORKSPACE_PATH env or --workspace flag global.
	ws := os.Getenv("WORKSPACE_PATH")
	if ws == "" {
		ws = WorkspacePath
	}
	if ws != "" {
		return base + "/" + strings.ReplaceAll(ws, "/", "-")
	}
	return base
}

// discoverClaudeCodeAgents reads agent definitions from CLAUDE_CLUTCH_CONFIG.
//
// Config format:
//
//	{
//	  "agents": [
//	    { "id": "dev", "agent_group": "developer", "workspace": "/path/to/project" }
//	  ]
//	}
//
// Returns nil for single-agent mode (falls back to AGENT_ID + WORKSPACE_PATH env vars).
func discoverClaudeCodeAgents() []agentDiscovery {
	configPath := os.Getenv("CLAUDE_CLUTCH_CONFIG")
	if configPath == "" {
		return nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("claude-code config not found at %s: %v", configPath, err)
		return nil
	}
	var cfg struct {
		Agents []struct {
			ID         string   `json:"id"`
			AgentGroup string   `json:"agent_group"`
			Workspace  string   `json:"workspace"`
			Subagents  []string `json:"subagents"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("claude-code config parse error: %v", err)
		return nil
	}
	var agents []agentDiscovery
	for _, a := range cfg.Agents {
		group := a.AgentGroup
		if group == "" {
			group = a.ID
		}
		agents = append(agents, agentDiscovery{
			ID:        a.ID,
			Group:     group,
			Workspace: a.Workspace,
			Subagents: a.Subagents,
		})
	}
	return agents
}
