package clutch

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// captureWriter buffers a handler's response so a local delegation's result can
// be both returned to the caller and inspected (to record the hop's outcome).
type captureWriter struct {
	hdr    http.Header
	status int
	buf    bytes.Buffer
}

func (c *captureWriter) Header() http.Header {
	if c.hdr == nil {
		c.hdr = http.Header{}
	}
	return c.hdr
}
func (c *captureWriter) WriteHeader(s int)            { c.status = s }
func (c *captureWriter) Write(b []byte) (int, error) { return c.buf.Write(b) }

func (c *captureWriter) flushTo(w http.ResponseWriter) {
	for k, vv := range c.hdr {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
	w.Write(c.buf.Bytes())
}

func (c *captureWriter) delegationState() string {
	if c.status >= 400 {
		return "failed"
	}
	var ar AskResponse
	if json.Unmarshal(c.buf.Bytes(), &ar) == nil && ar.Status == "error" {
		return "failed"
	}
	return "completed"
}

func handleDelegate(w http.ResponseWriter, r *http.Request) {
	if CpURL == "" {
		WriteJSON(w, 404, map[string]string{"error": "no control plane configured"})
		return
	}

	target := strings.TrimPrefix(r.URL.Path, "/delegate/")
	if target == "" {
		WriteJSON(w, 400, map[string]string{"error": "target name required"})
		return
	}

	if r.Method != http.MethodPost {
		WriteJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}

	var body map[string]any
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	if body == nil {
		body = map[string]any{}
	}

	message, _ := body["message"].(string)
	session, _ := body["session"].(string)
	async, _ := body["async"].(bool)
	if strings.TrimSpace(message) == "" {
		WriteJSON(w, 400, map[string]string{"error": "message required"})
		return
	}

	source := r.Header.Get("X-Clutch-Agent")

	// Link this hop to the task the source agent is currently serving (its
	// parent), so the request flow can be traced who-called-whom.
	parentTaskID := ""
	srcAgent := findLocalAgent(source)
	if srcAgent != nil {
		if v, ok := agentServingTask.Load(srcAgent.id); ok {
			parentTaskID, _ = v.(string)
		}
	}

	// Same-registration routing: if the target is an agent served by this clutch
	// instance, run it directly instead of round-tripping through the control
	// plane. Async delegations still go through the control plane so the caller
	// gets a trackable A2A task. The runtime session is namespaced the same way
	// the control plane names delegations — delegate:<source>:<session> — so
	// same-clutch and CP-routed hand-offs stay consistent.
	if !async {
		if agent := findLocalAgent(target); agent != nil {
			localSession := delegateSessionName(source, session)
			log.Printf("local delegation to %s (same-registration), session=%s, parent=%s", target, localSession, parentTaskID)
			cw := &captureWriter{}
			LocalDelegateAsk(cw, agent, message, localSession)
			cw.flushTo(w)
			srcFull := source
			if srcAgent != nil {
				srcFull = srcAgent.fullID
			}
			bufferActivity(map[string]any{
				"parentTaskId":  parentTaskID,
				"source":        source,
				"sourceAgentId": srcFull,
				"target":        target,
				"targetAgentId": agent.fullID,
				"session":       localSession,
				"runner":        Runner,
				"prompt":        message,
				"state":         cw.delegationState(),
				"ts":            time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
	}

	a2aBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      "clutch-" + timeNowID(),
		"method":  "message/send",
		"params": map[string]any{
			"message": map[string]any{
				"kind":      "message",
				"messageId": "msg_" + timeNowID(),
				"role":      "user",
				"parts": []map[string]string{{
					"kind": "text",
					"text": message,
				}},
				"metadata": map[string]any{
					"clawmatrix": map[string]any{
						"session":      session,
						"async":        async,
						"parentTaskId": parentTaskID,
					},
				},
			},
			"metadata": map[string]any{
				"clawmatrix": map[string]any{
					"session":      session,
					"async":        async,
					"parentTaskId": parentTaskID,
				},
			},
		},
	}

	resp, err := CpDoLongWithHeaders("POST", "/a2a/"+target, a2aBody, map[string]string{
		"X-Clutch-Agent": source,
	})
	if err != nil {
		WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}
}

// handleTaskStatus proxies task polling to the control plane so an agent can
// follow up on an async delegation. The caller delegated with async:true and
// got back a task id + statusUrl "/tasks/<id>"; this lets it poll that here.
// Routes: GET /tasks (list this agent's tasks) and GET /tasks/<id> (one task).
// The registration token + X-Clutch-Agent header satisfy the control plane's
// a2aAuth/canViewA2ATask checks.
func handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	if CpURL == "" {
		WriteJSON(w, 404, map[string]string{"error": "no control plane configured"})
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "/tasks" && r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	source := r.Header.Get("X-Clutch-Agent")
	resp, err := CpDoWithHeaders("GET", path, nil, map[string]string{
		"X-Clutch-Agent": source,
	})
	if err != nil {
		WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// delegateSessionName namespaces a local (same-clutch) delegation session as
// delegate:<source>:<hint>, matching how the control plane names agent→agent
// hand-offs so session ids are consistent regardless of routing.
func delegateSessionName(source, hint string) string {
	if hint == "" {
		hint = "default"
	}
	if source == "" {
		return "delegate:" + hint
	}
	return "delegate:" + source + ":" + hint
}

func timeNowID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	if CpURL == "" {
		WriteJSON(w, 404, map[string]string{"error": "no control plane configured"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	path := "/agent-connections"
	if agent := r.Header.Get("X-Clutch-Agent"); agent != "" {
		path += "?agent=" + agent
	}
	resp, err := CpDo("GET", path, nil)
	if err != nil {
		WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// --- Crons Proxy ---

func handleCrons(w http.ResponseWriter, r *http.Request) {
	if CpURL == "" {
		WriteJSON(w, 404, map[string]string{"error": "no control plane configured"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimSuffix(r.URL.Path, "/")

	switch {
	case path == "/crons" && r.Method == http.MethodPost:
		var req map[string]any
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				WriteJSON(w, 400, map[string]string{"error": "invalid JSON"})
				return
			}
		}
		if req == nil {
			req = map[string]any{}
		}
		if agent := r.Header.Get("X-Clutch-Agent"); agent != "" {
			req["agentName"] = agent
		}
		if localID, _ := req["agentId"].(string); localID != "" {
			if a := findLocalAgent(localID); a != nil {
				req["agentId"] = a.fullID
			}
		} else {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if a := findAgentByToken(tok); a != nil {
				req["agentId"] = a.fullID
			} else if def := getDefaultAgent(); def != nil {
				req["agentId"] = def.fullID
			}
		}

		resp, err := CpDo("POST", "/agent-crons", req)
		if err != nil {
			WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case path == "/crons" && r.Method == http.MethodGet:
		cronPath := "/agent-crons"
		if agent := r.Header.Get("X-Clutch-Agent"); agent != "" {
			cronPath += "?agentId=" + agent
		}
		resp, err := CpDo("GET", cronPath, nil)
		if err != nil {
			WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case strings.HasPrefix(path, "/crons/") && r.Method == http.MethodPut:
		cronID := strings.TrimPrefix(path, "/crons/")
		var req map[string]any
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				WriteJSON(w, 400, map[string]string{"error": "invalid JSON"})
				return
			}
		}
		resp, err := CpDo("PUT", "/agent-crons/"+cronID, req)
		if err != nil {
			WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case strings.HasPrefix(path, "/crons/") && r.Method == http.MethodDelete:
		cronID := strings.TrimPrefix(path, "/crons/")
		resp, err := CpDo("DELETE", "/agent-crons/"+cronID, nil)
		if err != nil {
			WriteJSON(w, 502, map[string]string{"error": "control plane unreachable"})
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	default:
		WriteJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

// findLocalAgent resolves a delegation target to an agent served by this clutch
// instance. The target may be a local id ("techlead-id") or a profile/group
// name ("techlead") — delegation targets use the latter. Matching is
// case-insensitive so callers (and LLMs) don't have to get the casing exact.
func findLocalAgent(target string) *RegisteredAgent {
	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if strings.EqualFold(RegisteredAgents[i].id, target) || strings.EqualFold(RegisteredAgents[i].group, target) {
			return &RegisteredAgents[i]
		}
	}
	return nil
}

func findAgentByToken(tok string) *RegisteredAgent {
	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if RegisteredAgents[i].agentToken == tok {
			return &RegisteredAgents[i]
		}
	}
	return nil
}

func getDefaultAgent() *RegisteredAgent {
	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if RegisteredAgents[i].isDefault {
			return &RegisteredAgents[i]
		}
	}
	if len(RegisteredAgents) > 0 {
		return &RegisteredAgents[0]
	}
	return nil
}
