package clutch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type AskRequest struct {
	Message string `json:"message"`
	Session string `json:"session"`
	Timeout int    `json:"timeout"`
}

type AskResponse struct {
	Status     string         `json:"status"`
	Response   string         `json:"response,omitempty"`
	Thinking   string         `json:"thinking,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"duration_ms"`
	Usage      map[string]any `json:"usage,omitempty"`
}

// handleAskForAgent handles /ask for a specific registered agent via subprocess.
func handleAskForAgent(w http.ResponseWriter, r *http.Request, agent *RegisteredAgent) {
	w.Header().Set("Content-Type", "application/json")

	if agent.agentCmd == "" {
		WriteJSON(w, 404, AskResponse{Status: "error", Error: "agent executor not configured"})
		return
	}

	var req AskRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSON(w, 400, AskResponse{Status: "error", Error: "invalid JSON"})
			return
		}
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		WriteJSON(w, 400, AskResponse{Status: "error", Error: "message is required"})
		return
	}

	session := req.Session
	if session == "" {
		session = "cli:default"
	}
	runner := newRunner()
	session = runner.NormalizeSession(agent.id, session)

	timeout := AgentTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	runSubprocess(w, r, runner, agent, msg, session, timeout)
}

// runSubprocess spawns the agent command and writes the result to w.
func runSubprocess(w http.ResponseWriter, r *http.Request, runner AgentRunner, agent *RegisteredAgent, msg, session string, timeout time.Duration) {
	if agent.agentCmd == "" {
		WriteJSON(w, 404, AskResponse{Status: "error", Error: "agent executor not configured"})
		return
	}

	runner.PrepareSession(agent, session)
	parts := runner.CommandArgs(agent, msg, session)

	// Serialize per-agent to prevent concurrent session file conflicts.
	agent.mu.Lock()
	defer agent.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	// WaitDelay force-closes stdout/stderr pipes 5s after the process exits.
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = runner.Env()
	if agent.workspace != "" {
		cmd.Dir = agent.workspace
	}
	if runner.UsesStdin() {
		cmd.Stdin = strings.NewReader(msg)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	if ctx.Err() == context.DeadlineExceeded {
		WriteJSON(w, 504, AskResponse{
			Status:     "timeout",
			Error:      fmt.Sprintf("agent did not respond within %s", timeout),
			DurationMs: durationMs,
		})
		return
	}

	if err != nil {
		errMsg := fmt.Sprintf("agent failed: %v", err)
		if stderr.Len() > 0 {
			errMsg += "\nstderr: " + stderr.String()
		}
		WriteJSON(w, 500, AskResponse{
			Status:     "error",
			Error:      errMsg,
			DurationMs: durationMs,
		})
		return
	}

	response, thinking, usage := runner.ParseOutput(stdout.String(), stderr.String())

	// Persist runner-specific session mapping for resumption.
	if claudeSessionID, _ := usage["session_id"].(string); claudeSessionID != "" {
		runner.StoreSession(agent.id, session, claudeSessionID)
	}

	if response == "" && thinking == "" {
		WriteJSON(w, 502, AskResponse{
			Status:     "error",
			Error:      "agent returned empty response",
			DurationMs: durationMs,
			Usage:      usage,
		})
		return
	}

	WriteJSON(w, 200, AskResponse{
		Status:     "ok",
		Response:   response,
		Thinking:   thinking,
		DurationMs: durationMs,
		Usage:      usage,
	})
}

// LocalDelegateAsk runs a local agent's command for same-instance delegation.
func LocalDelegateAsk(w http.ResponseWriter, r *http.Request, agent *RegisteredAgent) {
	var req AskRequest
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		WriteJSON(w, 400, map[string]string{"error": "message required"})
		return
	}

	fakeBody, _ := json.Marshal(req)
	fakeReq, _ := http.NewRequest("POST", "/ask/"+agent.id, bytes.NewReader(fakeBody))
	fakeReq.Header.Set("Content-Type", "application/json")

	handleAskForAgent(w, fakeReq, agent)
}

// --- shared helpers ---

// splitFields splits a command string into args (like strings.Fields).
func splitFields(s string) []string {
	return strings.Fields(s)
}

// envAll returns a copy of os.Environ() (used by runners that need the full env).
func envAll() []string {
	return append([]string(nil), os.Environ()...)
}

// trimSpace trims leading/trailing whitespace.
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
