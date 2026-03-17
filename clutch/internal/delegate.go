package clutch

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
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

	// Same-instance routing: check if target is a local agent
	if agent := findLocalAgent(target); agent != nil {
		log.Printf("local delegation to %s (same-instance)", target)
		LocalDelegateAsk(w, r, agent)
		return
	}

	// Remote: proxy via control plane
	var body map[string]any
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			WriteJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
	}

	resp, err := CpDoLong("POST", "/agent-chat/"+target, body)
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

func findLocalAgent(id string) *RegisteredAgent {
	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if RegisteredAgents[i].id == id {
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
