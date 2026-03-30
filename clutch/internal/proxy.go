package clutch

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func checkAgentAuth(r *http.Request) bool {
	if len(RegisteredAgents) == 0 {
		return true // standalone mode
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	tok := strings.TrimPrefix(auth, "Bearer ")

	RegisteredAgentsMu.RLock()
	defer RegisteredAgentsMu.RUnlock()
	for i := range RegisteredAgents {
		if tok == RegisteredAgents[i].agentToken {
			return true
		}
	}
	return false
}

func Handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Host == "" && r.Method != http.MethodConnect {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			agentRef := ""
			if def := getDefaultAgent(); def != nil {
				agentRef = def.fullID
			}
			fmt.Fprintf(w, `{"status":"ok","agentId":"%s","agents":%d}`, agentRef, len(RegisteredAgents))
			return
		}

		// /allowlist — returns current egress allowlist for PreToolUse hooks
		if r.URL.Path == "/allowlist" && r.Method == http.MethodGet {
			domains, _ := Allowlist.Load().([]string)
			if domains == nil {
				domains = []string{}
			}
			w.Header().Set("Content-Type", "application/json")
			enc := `{"domains":[`
			for i, d := range domains {
				if i > 0 {
					enc += ","
				}
				enc += `"` + d + `"`
			}
			enc += `]}`
			fmt.Fprint(w, enc)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/ask/") && r.Method == http.MethodPost {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			agentLocalID := strings.TrimPrefix(r.URL.Path, "/ask/")
			if agent := findLocalAgent(agentLocalID); agent != nil {
				handleAskForAgent(w, r, agent)
				return
			}
			http.Error(w, `{"status":"error","error":"agent not found"}`, 404)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/workspace/") && r.Method == http.MethodGet {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			rest := strings.TrimPrefix(r.URL.Path, "/workspace/")
			if rest == "locks" {
				handleWorkspaceLocks(w, r)
				return
			}
			parts := strings.SplitN(rest, "/", 2)
			agentLocalID := parts[0]
			if agent := findLocalAgent(agentLocalID); agent != nil {
				subPath := ""
				if len(parts) > 1 {
					subPath = parts[1]
				}
				if subPath == "locks" {
					handleWorkspaceLocksForAgent(w, r, agent)
				} else {
					handleWorkspaceForAgent(w, r, agent)
				}
				return
			}
			handleWorkspace(w, r)
			return
		}
		if r.URL.Path == "/workspace/locks" && (r.Method == http.MethodGet || r.Method == http.MethodPut) {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			handleWorkspaceLocks(w, r)
			return
		}
		if r.URL.Path == "/workspace" && r.Method == http.MethodGet {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			handleWorkspace(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/sessions/") && r.Method == http.MethodGet {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			agentLocalID := strings.TrimPrefix(r.URL.Path, "/sessions/")
			if agent := findLocalAgent(agentLocalID); agent != nil {
				handleSessionsForAgent(w, r, agent)
				return
			}
			handleSessions(w, r)
			return
		}
		if r.URL.Path == "/sessions" && r.Method == http.MethodGet {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			handleSessions(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/workspace/") && r.Method == http.MethodPut {
			if !checkAgentAuth(r) {
				http.Error(w, `{"status":"error","error":"unauthorized"}`, 401)
				return
			}
			rest := strings.TrimPrefix(r.URL.Path, "/workspace/")
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) == 2 && parts[1] == "locks" {
				if agent := findLocalAgent(parts[0]); agent != nil {
					handleWorkspaceLocksForAgent(w, r, agent)
					return
				}
			}
			handleWorkspaceLocks(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/delegate/") {
			handleDelegate(w, r)
			return
		}
		if r.URL.Path == "/connections" && r.Method == http.MethodGet {
			handleConnections(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/crons") {
			handleCrons(w, r)
			return
		}
		http.Error(w, "not a proxy request", 400)
		return
	}

	http.Error(w, "not a proxy request", 400)
}

func LoadLocalAllowlist(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("allowlist: %v", err)
	}
	var rules []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			rules = append(rules, strings.ToLower(line))
		}
	}
	Allowlist.Store(rules)
	log.Printf("loaded %d rules from %s", len(rules), path)
}

func detectEnv() map[string]string {
	env := map[string]string{}

	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount"); err == nil {
		env["runtime"] = "kubernetes"
		if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			env["namespace"] = string(ns)
		}
	} else if _, err := os.Stat("/.dockerenv"); err == nil {
		env["runtime"] = "docker"
	} else {
		env["runtime"] = "baremetal"
	}

	for _, kv := range [][2]string{
		{"POD_NAME", "podName"},
		{"POD_IP", "podIP"},
		{"NODE_NAME", "nodeName"},
	} {
		if v := os.Getenv(kv[0]); v != "" {
			env[kv[1]] = v
		}
	}

	if p := gcpMeta("project/project-id"); p != "" {
		env["cloud"] = "gcp"
		env["project"] = p
		if z := gcpMeta("instance/zone"); z != "" {
			parts := strings.Split(z, "/")
			env["zone"] = parts[len(parts)-1]
		}
		if c := gcpMeta("instance/attributes/cluster-name"); c != "" {
			env["cluster"] = c
		}
	}

	return env
}

func gcpMeta(path string) string {
	c := &http.Client{Timeout: 500 * time.Millisecond}
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/"+path, nil)
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := c.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
