package clutch

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

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

	// Same-registration routing: if the target is an agent served by this clutch
	// instance, run it directly instead of round-tripping through the control
	// plane. Async delegations still go through the control plane so the caller
	// gets a trackable A2A task.
	if !async {
		if agent := findLocalAgent(target); agent != nil {
			log.Printf("local delegation to %s (same-registration)", target)
			LocalDelegateAsk(w, r, agent)
			return
		}
	}

	source := r.Header.Get("X-Clutch-Agent")
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
						"session": session,
						"async":   async,
					},
				},
			},
			"metadata": map[string]any{
				"clawmatrix": map[string]any{
					"session": session,
					"async":   async,
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
// name ("techlead") — delegation targets use the latter.
func findLocalAgent(target string) *RegisteredAgent {
	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if RegisteredAgents[i].id == target || RegisteredAgents[i].group == target {
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
