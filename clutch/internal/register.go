package clutch

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

// GatewayVersion is set by main before calling Register.
var GatewayVersion string

func Register() {
	type agentReq struct {
		ID         string         `json:"id"`
		AgentGroup string         `json:"agent_group,omitempty"`
		Meta       map[string]any `json:"meta"`
	}
	type connReq struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}

	runner := newRunner()

	var agentsList []agentReq
	var connections []connReq
	discoveredMap := map[string]*agentDiscovery{}

	if PreferredAgentID != "" {
		if discovered := runner.DiscoverAgents(); len(discovered) > 0 {
			agentGroups := map[string]bool{}
			for i := range discovered {
				discoveredMap[discovered[i].ID] = &discovered[i]
				agentGroups[discovered[i].Group] = true
			}
			for _, a := range discovered {
				agentsList = append(agentsList, agentReq{
					ID:         a.ID,
					AgentGroup: a.Group,
					Meta: map[string]any{
						"chatUrl":      HostBaseURL + "/ask/" + a.ID,
						"workspaceUrl": HostBaseURL + "/workspace/" + a.ID,
						"sessionsUrl":  HostBaseURL + "/sessions/" + a.ID,
						"runner":       Runner,
					},
				})
				for _, targetGroup := range a.Subagents {
					if agentGroups[targetGroup] {
						connections = append(connections, connReq{Source: a.Group, Target: targetGroup})
					}
				}
			}
		}
	}

	if len(agentsList) == 0 {
		if PreferredAgentID == "" {
			log.Fatal("agent ID required: set AGENT_ID env or --agent-id flag")
		}
		meta := map[string]any{"runner": Runner}
		for _, kv := range [][2]string{
			{"AGENT_IMAGE", "image"},
			{"CHAT_URL", "chatUrl"},
			{"WORKSPACE_URL", "workspaceUrl"},
			{"SESSIONS_URL", "sessionsUrl"},
		} {
			if v := os.Getenv(kv[0]); v != "" {
				meta[kv[1]] = v
			}
		}
		if _, ok := meta["chatUrl"]; !ok && (AgentCmd != "" || runner.AgentCmd(PreferredAgentID) != "") {
			meta["chatUrl"] = HostBaseURL + "/ask/" + PreferredAgentID
		}
		if _, ok := meta["workspaceUrl"]; !ok && WorkspacePath != "" {
			meta["workspaceUrl"] = HostBaseURL + "/workspace/" + PreferredAgentID
		}
		sp := SessionsPath
		if sp == "" {
			sp = runner.SessionsPath(PreferredAgentID)
		}
		if _, ok := meta["sessionsUrl"]; !ok && sp != "" {
			meta["sessionsUrl"] = HostBaseURL + "/sessions/" + PreferredAgentID
		}
		dc := agentDiscovery{ID: PreferredAgentID, Group: PreferredAgentGroup, Default: true, Workspace: WorkspacePath}
		discoveredMap[PreferredAgentID] = &dc
		agentsList = []agentReq{{ID: PreferredAgentID, AgentGroup: PreferredAgentGroup, Meta: meta}}
	}

	body := map[string]any{
		"agents":      agentsList,
		"connections": connections,
		"environment": detectEnv(),
		"gateway": map[string]string{
			"version":   GatewayVersion,
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
			"startedAt": time.Now().UTC().Format(time.RFC3339),
		},
	}

	var resp *http.Response
	for {
		var err error
		resp, err = CpDo("POST", "/register", body)
		if err != nil {
			log.Fatalf("register: %v", err)
		}
		if resp.StatusCode == 409 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("register: 409 %s — retrying in 15s", b)
			time.Sleep(15 * time.Second)
			continue
		}
		break
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("register: %s %s", resp.Status, b)
	}

	var result struct {
		Agents []struct {
			AgentID           string `json:"agentId"`
			LocalID           string `json:"localId"`
			AgentGroup        string `json:"agentGroup"`
			Token             string `json:"token"`
			RegistrationToken string `json:"registrationToken"`
		} `json:"agents"`
		Allowlist  []string `json:"egressAllowlist"`
		TTLMinutes int      `json:"ttlMinutes"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	Allowlist.Store(result.Allowlist)
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		LastMod.Store(lm)
	}
	writeClaudeAllowlist(result.Allowlist)

	RegisteredAgentsMu.Lock()
	var finalAgents []RegisteredAgent
	for _, ag := range result.Agents {
		localID := ag.LocalID
		if localID == "" {
			localID = ag.AgentGroup
		}
		d := discoveredMap[localID]
		if d == nil {
			continue
		}
		regToken := ag.RegistrationToken
		if regToken == "" {
			regToken = CpToken
		}
		finalAgents = append(finalAgents, RegisteredAgent{
			id:                localID,
			fullID:            ag.AgentID,
			agentToken:        ag.Token,
			registrationToken: regToken,
			workspace:         d.Workspace,
			sessionsPath:      runner.SessionsPath(localID),
			agentCmd:          runner.AgentCmd(localID),
			isDefault:         localID == PreferredAgentID,
		})
	}
	RegisteredAgents = finalAgents
	RegisteredAgentsMu.Unlock()

	// Update global workspace/sessions paths from discovered config (runner-specific).
	if d := discoveredMap[PreferredAgentID]; d != nil && d.Workspace != "" {
		WorkspacePath = d.Workspace
		if SessionsPath == "" {
			SessionsPath = runner.SessionsPath(PreferredAgentID)
		}
	}

	log.Printf("registered %d agents (%d rules)", len(RegisteredAgents), len(result.Allowlist))
}

func DeregisterAll() {
	RegisteredAgentsMu.RLock()
	agents := RegisteredAgents
	RegisteredAgentsMu.RUnlock()

	for _, a := range agents {
		resp, err := CpDo("DELETE", "/register/"+a.fullID, nil)
		if err != nil {
			log.Printf("deregister %s: %v", a.fullID, err)
			continue
		}
		resp.Body.Close()
		log.Printf("deregistered: %s", a.fullID)
	}
}

func HeartbeatLoop() {
	tick := time.NewTicker(30 * time.Second)
	for range tick.C {
		count := Stats.ReqCount.Swap(0)
		totalMs := Stats.TotalMs.Swap(0)
		var avgMs int64
		if count > 0 {
			avgMs = totalMs / count
		}
		hbStats := map[string]int64{
			"allowed":  Stats.Allowed.Swap(0),
			"blocked":  Stats.Blocked.Swap(0),
			"avgMs":    avgMs,
			"minMs":    Stats.MinMs.Swap(0),
			"maxMs":    Stats.MaxMs.Swap(0),
			"reqCount": count,
		}

		RegisteredAgentsMu.RLock()
		agents := RegisteredAgents
		RegisteredAgentsMu.RUnlock()

		for _, a := range agents {
			body := map[string]any{
				"agentId": a.fullID,
				"stats":   hbStats,
			}
			resp, err := CpDoWithToken("POST", "/heartbeat", body, a.registrationToken)
			if err != nil {
				log.Printf("heartbeat %s: %v", a.fullID, err)
				continue
			}
			var result struct {
				Status string `json:"status"`
				Reason string `json:"reason"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			if result.Status == "kill" {
				log.Printf("kill signal for %s: %s — shutting down", a.fullID, result.Reason)
				DeregisterAll()
				os.Exit(0)
			}
		}
	}
}

func ConfigPollLoop() {
	tick := time.NewTicker(5 * time.Minute)
	for range tick.C {
		req, _ := http.NewRequest("GET", CpURL+"/config", nil)
		req.Header.Set("Authorization", "Bearer "+CpToken)
		if lm, ok := LastMod.Load().(string); ok && lm != "" {
			req.Header.Set("If-Modified-Since", lm)
		}

		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			log.Printf("config poll: %v", err)
			continue
		}

		if resp.StatusCode == 304 {
			resp.Body.Close()
			continue
		}

		if resp.StatusCode == 200 {
			var cfg struct {
				Allowlist []string `json:"egressAllowlist"`
			}
			json.NewDecoder(resp.Body).Decode(&cfg)
			resp.Body.Close()

			Allowlist.Store(cfg.Allowlist)
			if lm := resp.Header.Get("Last-Modified"); lm != "" {
				LastMod.Store(lm)
			}
			writeClaudeAllowlist(cfg.Allowlist)
			log.Printf("config updated: %d rules", len(cfg.Allowlist))
		} else {
			resp.Body.Close()
		}
	}
}

func LogFlushLoop() {
	tick := time.NewTicker(5 * time.Second)
	for range tick.C {
		flushLogs()
	}
}

func bufferLog(entry map[string]any) {
	LogBufMu.Lock()
	LogBuf = append(LogBuf, entry)
	n := len(LogBuf)
	LogBufMu.Unlock()

	if n >= 50 {
		go flushLogs()
	}
}

const maxLogBuf = 500

func flushLogs() {
	LogBufMu.Lock()
	if len(LogBuf) == 0 {
		LogBufMu.Unlock()
		return
	}
	batch := LogBuf
	LogBuf = nil
	LogBufMu.Unlock()

	body := map[string]any{
		"entries": batch,
	}
	resp, err := CpDo("POST", "/logs", body)
	if err != nil {
		log.Printf("log flush: %v (%d entries re-queued)", err, len(batch))
		requeueLogs(batch)
		return
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("log flush: %s %s (%d entries re-queued)", resp.Status, b, len(batch))
		requeueLogs(batch)
		return
	}
}

func requeueLogs(batch []map[string]any) {
	LogBufMu.Lock()
	LogBuf = append(batch, LogBuf...)
	if len(LogBuf) > maxLogBuf {
		LogBuf = LogBuf[len(LogBuf)-maxLogBuf:]
	}
	LogBufMu.Unlock()
}
