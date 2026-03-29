package clutch

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

// openclawRunner handles openclaw-specific subprocess execution.
// When AgentGatewayURL is set, agentCmd uses "openclaw agent --agent <id>"
// (no --local) which connects to the running gateway as a thin WS client —
// no pipe inheritance, no zombies.
type openclawRunner struct{}

func (o *openclawRunner) CommandArgs(agent *RegisteredAgent, msg, session string) []string {
	ocSession := strings.ReplaceAll(session, ":", "-")
	return append(splitFields(agent.agentCmd), "--session-id", ocSession, "--json", "-m", msg)
}

func (o *openclawRunner) UsesStdin() bool { return false }

func (o *openclawRunner) Env() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "HOME=") {
			continue
		}
		filtered = append(filtered, e)
	}
	return append(filtered, "HOME=/root")
}

func (o *openclawRunner) PrepareSession(agent *RegisteredAgent, session string) {
	ocSession := strings.ReplaceAll(session, ":", "-")
	// Try direct file first (subprocess mode stores as <sessionId>.jsonl).
	repairOpenclawSession(agent.sessionsPath, ocSession)
	// Also repair all sessions in the gateway's sessions.json (gateway mode
	// stores sessions under UUID filenames regardless of --session-id).
	repairAllGatewaySessions(agent.sessionsPath)
}

func (o *openclawRunner) ParseOutput(stdout, _ string) (string, string, map[string]any) {
	return parseOpenclawOutput(stdout)
}

func (o *openclawRunner) AgentCmd(localID string) string {
	if AgentGatewayURL != "" {
		return "openclaw agent --agent " + localID
	}
	return "openclaw agent --local --agent " + localID
}

func (o *openclawRunner) SessionsPath(localID string) string {
	return openclawSessionsPath(localID)
}

func (o *openclawRunner) DiscoverAgents() []agentDiscovery {
	return discoverOpenclawAgents()
}

func (o *openclawRunner) StoreSession(_, _, _ string)               {}
func (o *openclawRunner) NormalizeSession(_, session string) string { return session }

func (o *openclawRunner) ParseSessionLine(entry map[string]any) (string, string, bool) {
	return parseOpenclawSessionLine(entry)
}

// parseOpenclawSessionLine parses openclaw/generic JSONL session format:
// {"type":"message","message":{"role":"...","content":[{"type":"text","text":"..."}]}}
func parseOpenclawSessionLine(entry map[string]any) (string, string, bool) {
	if entry["type"] != "message" {
		return "", "", false
	}
	msg, _ := entry["message"].(map[string]any)
	if msg == nil {
		return "", "", false
	}
	role, _ := msg["role"].(string)
	var parts []string
	if blocks, ok := msg["content"].([]any); ok {
		for _, b := range blocks {
			if block, ok := b.(map[string]any); ok && block["type"] == "text" {
				if t, _ := block["text"].(string); strings.TrimSpace(t) != "" {
					parts = append(parts, t)
				}
			}
		}
	}
	content := strings.Join(parts, "\n")
	if role == "" || strings.TrimSpace(content) == "" {
		return "", "", false
	}
	return role, content, true
}

// --- openclaw agent discovery ---

// openclawAgentEntry is an internal representation of one agent in the openclaw config.
type openclawAgentEntry struct {
	ID        string
	Name      string
	Group     string
	Default   bool
	Workspace string
	Subagents []string
}

// openclawSessionsPath returns the sessions directory for a given openclaw agent local ID.
func openclawSessionsPath(localID string) string {
	stateDir := os.Getenv("OPENCLAW_STATE_DIR")
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = home + "/.openclaw"
	}
	return stateDir + "/agents/" + localID + "/sessions"
}

// discoverOpenclawAgents reads the openclaw config and returns all configured agents.
func discoverOpenclawAgents() []agentDiscovery {
	configPath := os.Getenv("OPENCLAW_CONFIG")
	if configPath == "" {
		configPath = "/root/.openclaw/openclaw.json"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("openclaw config not found at %s: %v", configPath, err)
		return nil
	}

	var cfg struct {
		Agents struct {
			List []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				AgentGroup string `json:"agent_group"`
				Default    bool   `json:"default"`
				Workspace  string `json:"workspace"`
				Subagents  struct {
					AllowAgents []string `json:"allowAgents"`
				} `json:"subagents"`
			} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("openclaw config parse error: %v", err)
		return nil
	}

	var agents []agentDiscovery
	for _, a := range cfg.Agents.List {
		group := a.AgentGroup
		if group == "" {
			group = a.Name
		}
		agents = append(agents, agentDiscovery{
			ID:        a.ID,
			Group:     group,
			Default:   a.Default,
			Workspace: a.Workspace,
			Subagents: a.Subagents.AllowAgents,
		})
	}
	return agents
}

// --- openclaw output parsing ---

type ocPayload struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

type ocMeta struct {
	AgentMeta struct {
		Usage map[string]any `json:"usage"`
	} `json:"agentMeta"`
}

func parseOpenclawOutput(raw string) (string, string, map[string]any) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}

	// Gateway mode: {"runId":...,"result":{"payloads":[...],"meta":{...}}}
	var gateway struct {
		Result struct {
			Payloads []ocPayload `json:"payloads"`
			Meta     ocMeta      `json:"meta"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &gateway); err == nil && len(gateway.Result.Payloads) > 0 {
		return extractOcPayloads(gateway.Result.Payloads, gateway.Result.Meta.AgentMeta.Usage)
	}

	// Local mode: {"payloads":[...],"meta":{...}}
	var local struct {
		Payloads []ocPayload `json:"payloads"`
		Meta     ocMeta      `json:"meta"`
	}
	if err := json.Unmarshal([]byte(raw), &local); err != nil {
		return raw, "", nil
	}
	return extractOcPayloads(local.Payloads, local.Meta.AgentMeta.Usage)
}

func extractOcPayloads(payloads []ocPayload, usage map[string]any) (string, string, map[string]any) {
	var texts, thoughts []string
	for _, p := range payloads {
		if p.Type == "thinking" {
			if t := strings.TrimSpace(p.Thinking); t != "" {
				thoughts = append(thoughts, t)
			}
		} else {
			if t := strings.TrimSpace(p.Text); t != "" {
				texts = append(texts, t)
			}
		}
	}
	var u map[string]any
	if len(usage) > 0 {
		u = usage
	}
	return strings.Join(texts, "\n"), strings.Join(thoughts, "\n\n"), u
}

// repairOpenclawSession sanitizes a session file before a new request:
//   1. Strips trailing empty assistant messages (stopReason=toolUse/error/stop
//      with empty content) — caused by 529 overload retries.
//   2. Removes consecutive same-role messages — caused when a 529 fail is
//      saved then the retry succeeds, leaving two consecutive assistant turns.
//      In that case, empty/error turn is dropped, keeping the successful one.
func repairOpenclawSession(sessionsPath, sessionID string) {
	if sessionsPath == "" || sessionID == "" {
		return
	}
	path := sessionsPath + "/" + sessionID + ".jsonl"
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	// Parse all lines into structured entries.
	type entry struct {
		raw     string
		role    string
		content []any
		stop    string
	}
	var entries []entry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			entries = append(entries, entry{raw: line})
			continue
		}
		msg, _ := obj["message"].(map[string]any)
		if msg == nil {
			entries = append(entries, entry{raw: line})
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].([]any)
		stop, _ := msg["stopReason"].(string)
		entries = append(entries, entry{raw: line, role: role, content: content, stop: stop})
	}

	// Step 1: strip trailing empty assistant messages.
	for len(entries) > 0 {
		e := entries[len(entries)-1]
		if e.role == "assistant" && len(e.content) == 0 &&
			(e.stop == "toolUse" || e.stop == "error" || e.stop == "stop") {
			entries = entries[:len(entries)-1]
			continue
		}
		break
	}

	// Step 2: remove consecutive same-role assistant pairs.
	// When two assistant entries are adjacent, drop the earlier empty/error one.
	changed := true
	for changed {
		changed = false
		for i := 1; i < len(entries); i++ {
			if entries[i-1].role == "assistant" && entries[i].role == "assistant" {
				// Drop the earlier one if it's empty; otherwise drop the earlier one anyway.
				entries = append(entries[:i-1], entries[i:]...)
				changed = true
				break
			}
		}
	}

	rebuilt := make([]string, 0, len(entries))
	for _, e := range entries {
		rebuilt = append(rebuilt, e.raw)
	}
	original := strings.TrimRight(string(data), "\n")
	repaired := strings.Join(rebuilt, "\n")
	if repaired == original {
		return
	}
	out := repaired
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0644); err == nil {
		log.Printf("repaired corrupted openclaw session: %s", sessionID)
	}
}

// repairAllGatewaySessions reads sessions.json in sessionsPath and repairs
// every JSONL session file listed there. Gateway mode stores sessions as UUIDs
// mapped via sessions.json regardless of the --session-id passed to the
// subprocess, so repairOpenclawSession alone cannot find them.
func repairAllGatewaySessions(sessionsPath string) {
	if sessionsPath == "" {
		return
	}
	sjPath := sessionsPath + "/sessions.json"
	data, err := os.ReadFile(sjPath)
	if err != nil {
		return // no sessions.json — not in gateway mode or no sessions yet
	}
	var sm map[string]struct {
		SessionFile string `json:"sessionFile"`
	}
	if err := json.Unmarshal(data, &sm); err != nil {
		return
	}
	for _, s := range sm {
		if s.SessionFile == "" {
			continue
		}
		repairJSONLFile(s.SessionFile)
	}
}

// repairJSONLFile is the path-aware version of repairOpenclawSession.
func repairJSONLFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	type entry struct {
		raw     string
		role    string
		content []any
		stop    string
	}
	var entries []entry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			entries = append(entries, entry{raw: line})
			continue
		}
		msg, _ := obj["message"].(map[string]any)
		if msg == nil {
			entries = append(entries, entry{raw: line})
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].([]any)
		stop, _ := msg["stopReason"].(string)
		entries = append(entries, entry{raw: line, role: role, content: content, stop: stop})
	}

	// Strip trailing empty assistant messages.
	for len(entries) > 0 {
		e := entries[len(entries)-1]
		if e.role == "assistant" && len(e.content) == 0 &&
			(e.stop == "toolUse" || e.stop == "error" || e.stop == "stop") {
			entries = entries[:len(entries)-1]
			continue
		}
		break
	}

	// Remove consecutive assistant pairs (retry artifacts).
	changed := true
	for changed {
		changed = false
		for i := 1; i < len(entries); i++ {
			if entries[i-1].role == "assistant" && entries[i].role == "assistant" {
				entries = append(entries[:i-1], entries[i:]...)
				changed = true
				break
			}
		}
	}

	rebuilt := make([]string, 0, len(entries))
	for _, e := range entries {
		rebuilt = append(rebuilt, e.raw)
	}
	original := strings.TrimRight(string(data), "\n")
	repaired := strings.Join(rebuilt, "\n")
	if repaired == original {
		return
	}
	out := repaired
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0644); err == nil {
		log.Printf("repaired corrupted openclaw gateway session: %s", path)
	}
}
