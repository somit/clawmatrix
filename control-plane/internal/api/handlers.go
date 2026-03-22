package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"control-plane/internal/database"
)

// CronScheduler is implemented by internal/cron.Scheduler.
// Defined as interface here to avoid circular imports.
type CronScheduler interface {
	AddJob(job *database.CronJob) error
	RemoveJob(cronJobID uint)
	UpdateJob(job *database.CronJob) error
	TriggerJob(job *database.CronJob)
	NextRunTime(cronJobID uint) *time.Time
}

type Handlers struct {
	hub       *Hub
	scheduler CronScheduler
	oidc      *OIDCConfig
}

type J = map[string]any

func NewHandlers(hub *Hub, scheduler CronScheduler, oidc *OIDCConfig) *Handlers {
	return &Handlers{hub: hub, scheduler: scheduler, oidc: oidc}
}

// --- Admin ---

func (h *Handlers) CreateRegistration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string            `json:"name"`
		Description     string            `json:"description"`
		EgressAllowlist []string          `json:"egressAllowlist"`
		Labels          map[string]string `json:"labels"`
		TTLMinutes      int               `json:"ttlMinutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respond(w, 400, J{"error": "name required"})
		return
	}

	if existing, _ := database.GetRegistrationByName(req.Name); existing.Name != "" {
		respond(w, 409, J{"error": "already exists"})
		return
	}

	token := genToken()
	reg, err := database.CreateRegistration(req.Name, req.Description, token, req.EgressAllowlist, req.Labels, req.TTLMinutes)
	if err != nil {
		respond(w, 500, J{"error": "failed to create"})
		return
	}

	logAction("REGISTRATION_CREATED", reg.Name, "")
	h.hub.Broadcast(Event{Type: "registration:created", Data: J{"name": reg.Name}})
	respond(w, 201, J{"name": reg.Name, "token": token})
}

func (h *Handlers) ListRegistrations(w http.ResponseWriter, r *http.Request) {
	regs, err := database.ListRegistrations()
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	out := make([]J, 0, len(regs))
	for _, t := range regs {
		count, _ := database.CountHealthyAgents(t.Name)
		monitoringEnabled := t.MonitorLastSeen != nil && time.Since(*t.MonitorLastSeen) < 2*time.Minute
		out = append(out, J{
			"name":              t.Name,
			"description":       t.Description,
			"egressAllowlist":   database.GetAllowlist(&t),
			"labels":            database.GetLabels(&t),
			"ttlMinutes":        t.TTLMinutes,
			"agents":            count,
			"totalRegistered":   t.TotalRegistered,
			"archived":          t.Archived,
			"updatedAt":         t.UpdatedAt.Format(time.RFC3339),
			"monitoringEnabled": monitoringEnabled,
		})
	}
	respond(w, 200, out)
}

func (h *Handlers) UpdateRegistration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Name            *string           `json:"name"`
		Description     *string           `json:"description"`
		EgressAllowlist []string          `json:"egressAllowlist"`
		Labels          map[string]string `json:"labels"`
		TTLMinutes      *int              `json:"ttlMinutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil && *req.Name != "" {
		if *req.Name != name {
			if existing, _ := database.GetRegistrationByName(*req.Name); existing.Name != "" {
				respond(w, 409, J{"error": "name already taken"})
				return
			}
		}
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.EgressAllowlist != nil {
		al, _ := json.Marshal(req.EgressAllowlist)
		updates["egress_allowlist"] = string(al)
	}
	if req.Labels != nil {
		lb, _ := json.Marshal(req.Labels)
		updates["labels"] = string(lb)
	}
	if req.TTLMinutes != nil {
		updates["ttl_minutes"] = *req.TTLMinutes
	}

	if len(updates) == 0 {
		respond(w, 400, J{"error": "nothing to update"})
		return
	}

	if err := database.UpdateRegistration(name, updates); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	logAction("REGISTRATION_UPDATED", name, fmt.Sprintf("%v", updates))
	h.hub.Broadcast(Event{Type: "registration:updated", Data: J{"name": name}})
	respond(w, 200, J{"status": "updated"})
}

func (h *Handlers) DeleteRegistration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	reg, err := database.GetRegistrationByName(name)
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	if reg.TotalRegistered > 0 {
		respond(w, 409, J{"error": "cannot delete — agents have registered with this registration. Use archive instead."})
		return
	}
	if err := database.DeleteRegistration(name); err != nil {
		respond(w, 500, J{"error": "failed to delete"})
		return
	}
	logAction("REGISTRATION_DELETED", name, "")
	respond(w, 200, J{"status": "deleted"})
}

func (h *Handlers) ArchiveRegistration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}
	if err := database.SetArchived(name, req.Archived); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	action := "REGISTRATION_ARCHIVED"
	if !req.Archived {
		action = "REGISTRATION_UNARCHIVED"
	}
	logAction(action, name, "")
	respond(w, 200, J{"status": "ok", "archived": req.Archived})
}

// --- Sidecar ---

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)

	var req struct {
		// New format: flat agents list
		Agents []struct {
			ID         string          `json:"id"`
			AgentGroup string          `json:"agent_group"` // role/group name (optional, defaults to id)
			Meta       json.RawMessage `json:"meta"`
		} `json:"agents"`
		Connections []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"connections"`
		Environment json.RawMessage `json:"environment"`
		Gateway     json.RawMessage `json:"gateway"`

		// Legacy format: primary + subagents
		ID        string          `json:"id"`
		Meta      json.RawMessage `json:"meta"`
		Subagents []struct {
			ID   string          `json:"id"`
			Meta json.RawMessage `json:"meta"`
		} `json:"subagents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	// Normalize: convert legacy format to agents list
	type agentEntry struct {
		id    string
		group string // role/group name
		meta  json.RawMessage
	}
	var agentEntries []agentEntry

	if len(req.Agents) > 0 {
		// New format
		for _, a := range req.Agents {
			if a.ID != "" {
				group := a.AgentGroup
				if group == "" {
					group = a.ID
				}
				agentEntries = append(agentEntries, agentEntry{id: a.ID, group: group, meta: a.Meta})
			}
		}
	} else if req.ID != "" {
		// Legacy format: primary + subagents
		agentEntries = append(agentEntries, agentEntry{id: req.ID, group: req.ID, meta: req.Meta})
		for _, sub := range req.Subagents {
			if sub.ID != "" && sub.ID != req.ID {
				agentEntries = append(agentEntries, agentEntry{id: sub.ID, group: sub.ID, meta: sub.Meta})
			}
		}
	}

	if len(agentEntries) == 0 {
		respond(w, 400, J{"error": "at least one agent required"})
		return
	}

	now := time.Now().UTC()
	isFirstAgent := true
	var agents []J

	for _, entry := range agentEntries {
		result, err := h.registerOneAgent(reg, entry.id, entry.group, req.Environment, entry.meta, req.Gateway, now, !isFirstAgent)
		if err != nil {
			if isFirstAgent {
				respond(w, result.statusCode, J{"error": err.Error()})
				return
			}
			log.Printf("agent registration failed for %s: %v", entry.id, err)
			continue
		}
		agents = append(agents, result.toJSON())
		isFirstAgent = false
	}

	// Create connections from request
	for _, c := range req.Connections {
		if c.Source != "" && c.Target != "" && c.Source != c.Target {
			if !database.HasConnection(c.Source, c.Target) {
				database.CreateConnection(c.Source, c.Target)
			}
		}
	}

	firstAgentID := agentEntries[0].id
	logAction("REGISTERED", fmt.Sprintf("%s-%s", reg.Name, firstAgentID), reg.Name)
	h.hub.Broadcast(Event{Type: "agent:registered", Data: J{"registration": reg.Name, "agents": len(agents)}})

	w.Header().Set("Last-Modified", reg.UpdatedAt.UTC().Format(http.TimeFormat))
	respond(w, 201, J{
		"agents":     agents,
		"egressAllowlist": database.GetAllowlist(reg),
		"ttlMinutes": reg.TTLMinutes,
	})
}

type registerResult struct {
	agentID      string
	agentLocalID string // the id field sent by clutch, used for discoveredMap lookup
	agentGroup   string // role/group name
	agentToken   string
	statusCode   int
}

func (r *registerResult) toJSON() J {
	return J{
		"agentId":    r.agentID,
		"localId":    r.agentLocalID,
		"agentGroup": r.agentGroup,
		"token":      r.agentToken,
	}
}

func (h *Handlers) registerOneAgent(reg *database.Registration, agentLocalID, agentGroup string, env, meta, gw json.RawMessage, now time.Time, isSubagent bool) (*registerResult, error) {
	id := fmt.Sprintf("%s-%s", reg.Name, agentLocalID)

	// Upsert AgentProfile — create if not exists, mark source as automatic
	database.UpsertAgentProfile(agentGroup, reg.Name, "automatic")

	// Check if this agent already exists (re-registration after restart)
	existing, err := database.GetAgent(id)
	isReRegister := err == nil

	if isReRegister && existing.Status == "healthy" {
		staleThreshold := 90 * time.Second
		if time.Since(existing.LastHeartbeat) <= staleThreshold {
			return &registerResult{statusCode: 409}, fmt.Errorf("agent %s is already healthy", id)
		}
	}

	// 1 registration = 1 runner (gateway). Only check for primary agent — subagents
	// share the same registration and are part of the same openclaw instance.
	if !isSubagent && !isReRegister {
		count, _ := database.CountHealthyAgents(reg.Name)
		if count >= 1 {
			return &registerResult{statusCode: 429}, fmt.Errorf("registration %s already has an active runner", reg.Name)
		}
	}

	agentToken := genAgentToken()

	if isReRegister {
		updates := map[string]any{
			"agent_profile": agentGroup,
			"token":         agentToken,
			"environment":   string(env),
			"meta":          string(meta),
			"gateway":       string(gw),
			"registered_at": now,
			"last_heartbeat": now,
			"status":        "healthy",
			"kill_reason":   "",
		}
		database.DB.Model(&database.Agent{}).Where("id = ?", id).Updates(updates)
	} else {
		agent := &database.Agent{
			ID:           id,
			AgentProfile: &agentGroup,
			Token:        agentToken,
			Status:           "healthy",
			Environment:      string(env),
			Meta:             string(meta),
			Gateway:          string(gw),
			RegisteredAt:     now,
			LastHeartbeat:    now,
		}
		if err := database.CreateAgent(agent); err != nil {
			return &registerResult{statusCode: 500}, fmt.Errorf("failed to register")
		}
		database.IncrementRegistrations(reg.Name)
	}

	return &registerResult{
		agentID:      id,
		agentLocalID: agentLocalID,
		agentGroup:   agentGroup,
		agentToken:   agentToken,
		statusCode:   201,
	}, nil
}


func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agentId"`
		Stats   struct {
			Allowed  int64 `json:"allowed"`
			Blocked  int64 `json:"blocked"`
			AvgMs    int64 `json:"avgMs"`
			MinMs    int64 `json:"minMs"`
			MaxMs    int64 `json:"maxMs"`
			ReqCount int64 `json:"reqCount"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	agent, err := database.GetAgent(req.AgentID)
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	s := database.HeartbeatStats{
		Allowed:  req.Stats.Allowed,
		Blocked:  req.Stats.Blocked,
		AvgMs:    req.Stats.AvgMs,
		MinMs:    req.Stats.MinMs,
		MaxMs:    req.Stats.MaxMs,
		ReqCount: req.Stats.ReqCount,
	}
	database.UpdateAgentHeartbeat(req.AgentID, s)
	database.RecordMetric(req.AgentID, database.GetAgentRegistrationName(agent), s)

	if agent.Status == "kill" {
		respond(w, 200, J{"status": "kill", "reason": agent.KillReason})
		return
	}

	if agent.Status == "stale" {
		database.UpdateAgentStatus(req.AgentID, "healthy", "")
		logAction("RECOVERED", req.AgentID, "")
		h.hub.Broadcast(Event{Type: "agent:recovered", Data: J{"id": req.AgentID}})
	}

	respond(w, 200, J{"status": "ok"})
}

func (h *Handlers) IngestLogs(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)

	var req struct {
		Entries []struct {
			Domain    string `json:"domain"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Action    string `json:"action"`
			Status    int    `json:"status"`
			LatencyMs int64  `json:"latencyMs"`
			Ts        string `json:"ts"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	logs := make([]database.RequestLog, 0, len(req.Entries))
	for _, e := range req.Entries {
		ts, _ := time.Parse(time.RFC3339, e.Ts)
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		logs = append(logs, database.RequestLog{
			RegistrationName: reg.Name,
			Domain:           e.Domain,
			Method:           e.Method,
			Path:             e.Path,
			Action:           e.Action,
			StatusCode:       e.Status,
			LatencyMs:        e.LatencyMs,
			CreatedAt:        ts,
		})
	}

	if err := database.InsertRequestLogs(logs); err != nil {
		respond(w, 500, J{"error": "failed to store"})
		return
	}
	h.hub.Broadcast(Event{Type: "log:batch", Data: J{"registration": reg.Name, "count": len(logs)}})
	respond(w, 200, J{"accepted": len(logs)})
}

func (h *Handlers) QueryLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := time.Now().UTC().Add(-1 * time.Hour)
	if s := q.Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = time.Now().UTC().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}

	logs, err := database.QueryRequestLogs(
		q.Get("type"), q.Get("agent"), q.Get("action"), q.Get("domain"),
		since, 200, 0,
	)
	if err != nil {
		respond(w, 500, J{"error": "failed to query"})
		return
	}

	out := make([]J, 0, len(logs))
	for _, l := range logs {
		out = append(out, J{
			"registration": l.RegistrationName,
			"domain":       l.Domain,
			"method":       l.Method,
			"path":         l.Path,
			"action":       l.Action,
			"status":       l.StatusCode,
			"latencyMs":    l.LatencyMs,
			"ts":           l.CreatedAt.Format(time.RFC3339),
		})
	}
	respond(w, 200, out)
}

func (h *Handlers) LogStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := time.Now().UTC().Add(-24 * time.Hour)
	if s := q.Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = time.Now().UTC().Add(-d)
		}
	}

	rows, err := database.QueryLogStats(q.Get("type"), since)
	if err != nil {
		respond(w, 500, J{"error": "failed to query"})
		return
	}

	type stat struct {
		Domain       string `json:"domain"`
		Registration string `json:"registration"`
		Total        int64  `json:"total"`
		Allowed      int64  `json:"allowed"`
		Blocked      int64  `json:"blocked"`
		LastSeen     string `json:"lastSeen"`
	}
	byKey := map[string]*stat{}
	for _, row := range rows {
		key := row.Domain + "|" + row.RegistrationName
		if _, ok := byKey[key]; !ok {
			byKey[key] = &stat{Domain: row.Domain, Registration: row.RegistrationName}
		}
		d := byKey[key]
		d.Total += row.Count
		if row.Action == "allowed" {
			d.Allowed += row.Count
		} else if row.Action == "blocked" {
			d.Blocked += row.Count
		}
		if d.LastSeen == "" || row.LastSeen > d.LastSeen {
			d.LastSeen = row.LastSeen
		}
	}

	out := make([]*stat, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	respond(w, 200, out)
}

func (h *Handlers) Deregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := database.DeleteAgent(id); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	logAction("DEREGISTERED", id, "")
	h.hub.Broadcast(Event{Type: "agent:killed", Data: J{"id": id}})
	respond(w, 200, J{"status": "deregistered"})
}

func (h *Handlers) MonitorHeartbeat(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)
	if err := database.TouchMonitorHeartbeat(reg.Name); err != nil {
		respond(w, 500, J{"error": "db error"})
		return
	}
	respond(w, 200, J{"status": "ok"})
}

func (h *Handlers) Config(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)

	lastMod := reg.UpdatedAt.UTC()
	w.Header().Set("Last-Modified", lastMod.Format(http.TimeFormat))

	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if parsed, err := http.ParseTime(ims); err == nil {
			if !lastMod.Truncate(time.Second).After(parsed.Truncate(time.Second)) {
				w.WriteHeader(304)
				return
			}
		}
	}

	respond(w, 200, J{
		"egressAllowlist": database.GetAllowlist(reg),
		"updatedAt":       reg.UpdatedAt.Format(time.RFC3339),
	})
}

// --- Query ---

func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	agents, err := database.ListAgents(typeFilter)
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	u := userFromCtx(r)
	visibleProfiles, isAdmin := database.VisibleProfiles(u.ID)
	if !isAdmin {
		profileSet := make(map[string]bool, len(visibleProfiles))
		for _, p := range visibleProfiles {
			profileSet[p] = true
		}
		filtered := agents[:0]
		for _, a := range agents {
			if a.AgentProfile != nil && profileSet[*a.AgentProfile] {
				filtered = append(filtered, a)
			}
		}
		agents = filtered
	}

	out := make([]J, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentToJSON(&a))
	}
	respond(w, 200, out)
}

func (h *Handlers) GetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := database.GetAgent(id)
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	if agent.AgentProfile != nil {
		if !checkProfilePerm(w, r, *agent.AgentProfile, database.PermViewAgents) {
			return
		}
	}
	respond(w, 200, agentToJSON(agent))
}

func (h *Handlers) GetMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typeName := q.Get("type")
	agentID := q.Get("agent")
	sinceStr := q.Get("since")

	since := time.Now().UTC().Add(-1 * time.Hour) // default: last hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = time.Now().UTC().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	limit := 500
	metrics, err := database.GetMetrics(typeName, agentID, since, limit)
	if err != nil {
		respond(w, 500, J{"error": "failed to query"})
		return
	}

	out := make([]J, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, J{
			"agentId":      m.AgentID,
			"registration": m.RegistrationName,
			"allowed":      m.Allowed,
			"blocked":      m.Blocked,
			"avgMs":        m.AvgMs,
			"minMs":        m.MinMs,
			"maxMs":        m.MaxMs,
			"reqCount":     m.ReqCount,
			"ts":           m.CreatedAt.Format(time.RFC3339),
		})
	}
	respond(w, 200, out)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	agents, _ := database.ListAgents("")
	total, healthy, stale := 0, 0, 0
	for _, a := range agents {
		total++
		if a.Status == "healthy" {
			healthy++
		}
		if a.Status == "stale" {
			stale++
		}
	}
	respond(w, 200, J{"status": "ok", "agents": total, "healthy": healthy, "stale": stale})
}

// --- Audit Events ---

func (h *Handlers) QueryAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := time.Now().UTC().Add(-24 * time.Hour)
	if s := q.Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = time.Now().UTC().Add(-d)
		}
	}
	limit := 50
	events, err := database.QueryAuditEvents(since, limit)
	if err != nil {
		respond(w, 500, J{"error": "failed to query"})
		return
	}
	out := make([]J, 0, len(events))
	for _, e := range events {
		var data any
		json.Unmarshal([]byte(e.Data), &data)
		out = append(out, J{
			"type": e.EventType,
			"data": data,
			"ts":   e.CreatedAt.Format(time.RFC3339),
		})
	}
	respond(w, 200, out)
}

// --- Metrics Series ---

func (h *Handlers) GetMetricsSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typeName := q.Get("type")
	agentID := q.Get("agent")
	sinceStr := q.Get("since")
	bucketStr := q.Get("bucket")

	since := time.Now().UTC().Add(-1 * time.Hour)
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = time.Now().UTC().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	bucketMin := 5
	if bucketStr != "" {
		if d, err := time.ParseDuration(bucketStr); err == nil {
			bucketMin = int(d.Minutes())
			if bucketMin < 1 {
				bucketMin = 1
			}
		}
	}

	metrics, err := database.GetMetrics(typeName, agentID, since, 0)
	if err != nil {
		respond(w, 500, J{"error": "failed to query"})
		return
	}

	// Bucket metrics in Go
	type bucket struct {
		Ts      time.Time `json:"ts"`
		Allowed int64     `json:"allowed"`
		Blocked int64     `json:"blocked"`
		AvgMs   int64     `json:"avgMs"`
		count   int64
		totalMs int64
	}

	buckets := map[int64]*bucket{}
	bucketDur := time.Duration(bucketMin) * time.Minute

	for _, m := range metrics {
		key := m.CreatedAt.Unix() / int64(bucketDur.Seconds())
		b, ok := buckets[key]
		if !ok {
			ts := time.Unix(key*int64(bucketDur.Seconds()), 0).UTC()
			b = &bucket{Ts: ts}
			buckets[key] = b
		}
		b.Allowed += m.Allowed
		b.Blocked += m.Blocked
		b.totalMs += m.AvgMs
		b.count++
	}

	// Sort by time
	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	out := make([]J, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		avgMs := int64(0)
		if b.count > 0 {
			avgMs = b.totalMs / b.count
		}
		out = append(out, J{
			"ts":      b.Ts.Format(time.RFC3339),
			"allowed": b.Allowed,
			"blocked": b.Blocked,
			"avgMs":   avgMs,
		})
	}
	respond(w, 200, out)
}

// --- Chat Proxy ---

const (
	sessionDefaultChat     = "autobot-manager-default-chat"
	sessionWorkspaceEditor = "autobot-manager-workspace-editor-chat"
)

var chatClient = &http.Client{Timeout: 10 * time.Minute}

func (h *Handlers) ChatProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Message string `json:"message"`
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		respond(w, 400, J{"error": "message required"})
		return
	}

	agent, err := database.GetAgent(id)
	if err != nil {
		respond(w, 404, J{"error": "agent not found"})
		return
	}
	if agent.AgentProfile != nil {
		if !checkProfilePerm(w, r, *agent.AgentProfile, database.PermChatWithAgents) {
			return
		}
	}

	// Parse chatUrl from agent's meta
	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	chatURL, _ := meta["chatUrl"].(string)
	if chatURL == "" {
		respond(w, 400, J{"error": "agent has no chat endpoint"})
		return
	}

	// Validate URL scheme
	parsed, err := url.Parse(chatURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respond(w, 400, J{"error": "invalid chat URL scheme"})
		return
	}

	msg := req.Message
	if req.Session == sessionWorkspaceEditor {
		msg = "[workspace-editor] " + msg
	}

	payload := J{"message": msg}
	if req.Session != "" {
		payload["session"] = req.Session
	}
	body, _ := json.Marshal(payload)
	proxyReq, err := http.NewRequestWithContext(context.Background(), "POST", chatURL, bytes.NewReader(body))
	if err != nil {
		respond(w, 500, J{"error": "failed to create request"})
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Accept", "text/event-stream, application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+agent.Token)

	resp, err := chatClient.Do(proxyReq)
	if err != nil {
		respond(w, 502, J{"error": "chat endpoint unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	flusher, canFlush := w.(http.Flusher)

	// Stream-through: relay whatever the upstream sends
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)

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

// --- Workspace Proxy ---

func (h *Handlers) WorkspaceProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	agent, err := database.GetAgent(id)
	if err != nil {
		respond(w, 404, J{"error": "agent not found"})
		return
	}
	if agent.AgentProfile != nil {
		if !checkProfilePerm(w, r, *agent.AgentProfile, database.PermViewAgents) {
			return
		}
	}

	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	wsURL, _ := meta["workspaceUrl"].(string)
	if wsURL == "" {
		respond(w, 400, J{"error": "agent has no workspace endpoint"})
		return
	}

	parsed, err := url.Parse(wsURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respond(w, 400, J{"error": "invalid workspace URL scheme"})
		return
	}

	// Forward query params
	if qs := r.URL.RawQuery; qs != "" {
		wsURL += "?" + qs
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), "GET", wsURL, nil)
	if err != nil {
		respond(w, 500, J{"error": "failed to create request"})
		return
	}
	proxyReq.Header.Set("Authorization", "Bearer "+agent.Token)

	resp, err := chatClient.Do(proxyReq)
	if err != nil {
		respond(w, 502, J{"error": "workspace endpoint unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// --- Workspace Locks Proxy ---

func (h *Handlers) WorkspaceLocksProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	agent, err := database.GetAgent(id)
	if err != nil {
		respond(w, 404, J{"error": "agent not found"})
		return
	}
	if agent.AgentProfile != nil {
		perm := database.PermViewAgents
		if r.Method == http.MethodPut {
			perm = database.PermConfigureAgents
		}
		if !checkProfilePerm(w, r, *agent.AgentProfile, perm) {
			return
		}
	}

	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	wsURL, _ := meta["workspaceUrl"].(string)
	if wsURL == "" {
		respond(w, 400, J{"error": "agent has no workspace endpoint"})
		return
	}

	parsed, err := url.Parse(wsURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respond(w, 400, J{"error": "invalid workspace URL scheme"})
		return
	}

	// Derive locks URL from workspace URL: /workspace -> /workspace/locks
	locksURL := strings.TrimSuffix(wsURL, "/") + "/locks"

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, locksURL, r.Body)
	if err != nil {
		respond(w, 500, J{"error": "failed to create request"})
		return
	}
	proxyReq.Header.Set("Authorization", "Bearer "+agent.Token)
	if r.Body != nil {
		proxyReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := chatClient.Do(proxyReq)
	if err != nil {
		respond(w, 502, J{"error": "workspace endpoint unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// --- Sessions Proxy ---

func (h *Handlers) SessionsProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	agent, err := database.GetAgent(id)
	if err != nil {
		respond(w, 404, J{"error": "agent not found"})
		return
	}
	if agent.AgentProfile != nil {
		if !checkProfilePerm(w, r, *agent.AgentProfile, database.PermViewAgents) {
			return
		}
	}

	var meta map[string]any
	json.Unmarshal([]byte(agent.Meta), &meta)
	sessURL, _ := meta["sessionsUrl"].(string)
	if sessURL == "" {
		respond(w, 400, J{"error": "agent has no sessions endpoint"})
		return
	}

	parsed, err := url.Parse(sessURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		respond(w, 400, J{"error": "invalid sessions URL scheme"})
		return
	}

	if qs := r.URL.RawQuery; qs != "" {
		sessURL += "?" + qs
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), "GET", sessURL, nil)
	if err != nil {
		respond(w, 500, J{"error": "failed to create request"})
		return
	}
	proxyReq.Header.Set("Authorization", "Bearer "+agent.Token)

	resp, err := chatClient.Do(proxyReq)
	if err != nil {
		respond(w, 502, J{"error": "sessions endpoint unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// --- Crons ---

func (h *Handlers) CreateCron(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string  `json:"name"`
		Description      string  `json:"description"`
		AgentID          string  `json:"agentId"`
		RegistrationName string  `json:"registrationName"`
		Schedule         string  `json:"schedule"`
		RunAt            *string `json:"runAt"`
		Timezone         string  `json:"timezone"`
		Session          string  `json:"session"`
		Message          string  `json:"message"`
		Enabled          *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Name == "" || req.AgentID == "" || req.Message == "" {
		respond(w, 400, J{"error": "name, agentId, and message required"})
		return
	}
	if req.Schedule == "" && req.RunAt == nil {
		respond(w, 400, J{"error": "either schedule or runAt is required"})
		return
	}
	if req.Schedule != "" && req.RunAt != nil {
		respond(w, 400, J{"error": "cannot set both schedule and runAt"})
		return
	}

	// Parse runAt if provided
	var runAtTime *time.Time
	if req.RunAt != nil {
		t, err := time.Parse(time.RFC3339, *req.RunAt)
		if err != nil {
			respond(w, 400, J{"error": "runAt must be RFC3339 format, e.g. 2026-03-01T10:00:00+05:30"})
			return
		}
		runAtTime = &t
	}

	// Verify the agent profile exists
	if _, err := database.GetAgentProfile(req.AgentID); err != nil {
		respond(w, 400, J{"error": "agent profile not found: " + req.AgentID})
		return
	}

	tz := "UTC"
	if req.Timezone != "" {
		tz = req.Timezone
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	job := &database.CronJob{
		Name:             req.Name,
		Description:      req.Description,
		AgentProfileName: req.AgentID,
		RegistrationName: req.RegistrationName,
		Schedule:         req.Schedule,
		RunAt:            runAtTime,
		Timezone:         tz,
		Session:          req.Session,
		Message:          req.Message,
		Enabled:          enabled,
	}

	if err := database.CreateCronJob(job); err != nil {
		respond(w, 500, J{"error": "failed to create cron"})
		return
	}

	if job.Enabled {
		if err := h.scheduler.AddJob(job); err != nil {
			respond(w, 400, J{"error": "invalid schedule: " + err.Error()})
			database.DeleteCronJob(job.ID)
			return
		}
	}

	h.hub.Broadcast(Event{Type: "cron:created", Data: J{"id": job.ID, "name": job.Name}})
	respond(w, 201, cronToJSON(job, h.scheduler))
}

func (h *Handlers) ListCrons(w http.ResponseWriter, r *http.Request) {
	registrationName := r.URL.Query().Get("type")
	var jobs []database.CronJob
	var err error
	if registrationName != "" {
		jobs, err = database.ListCronJobsByRegistration(registrationName)
	} else {
		jobs, err = database.ListCronJobs("")
	}
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	u := userFromCtx(r)
	visibleProfiles, isAdmin := database.VisibleProfiles(u.ID)
	if !isAdmin {
		profileSet := make(map[string]bool, len(visibleProfiles))
		for _, p := range visibleProfiles {
			profileSet[p] = true
		}
		filtered := jobs[:0]
		for _, j := range jobs {
			if profileSet[j.AgentProfileName] {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}

	out := make([]J, 0, len(jobs))
	for i := range jobs {
		out = append(out, cronToJSON(&jobs[i], h.scheduler))
	}
	respond(w, 200, out)
}

func (h *Handlers) GetCron(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}
	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	if job.AgentProfileName != "" {
		if !checkProfilePerm(w, r, job.AgentProfileName, database.PermViewCrons) {
			return
		}
	}
	respond(w, 200, cronToJSON(job, h.scheduler))
}

func (h *Handlers) UpdateCron(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		AgentID          *string `json:"agentId"`
		RegistrationName *string `json:"registrationName"`
		Schedule         *string `json:"schedule"`
		RunAt            *string `json:"runAt"` // RFC3339 datetime; send empty string to clear
		Timezone         *string `json:"timezone"`
		Session          *string `json:"session"`
		Message          *string `json:"message"`
		Enabled          *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AgentID != nil {
		updates["agent_profile_name"] = *req.AgentID
	}
	if req.RegistrationName != nil {
		updates["registration_name"] = *req.RegistrationName
	}
	if req.Schedule != nil {
		updates["schedule"] = *req.Schedule
	}
	if req.RunAt != nil {
		if *req.RunAt == "" {
			updates["run_at"] = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.RunAt)
			if err != nil {
				respond(w, 400, J{"error": "runAt must be RFC3339 format"})
				return
			}
			updates["run_at"] = t
		}
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.Session != nil {
		updates["session"] = *req.Session
	}
	if req.Message != nil {
		updates["message"] = *req.Message
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if len(updates) == 0 {
		respond(w, 400, J{"error": "nothing to update"})
		return
	}

	if err := database.UpdateCronJob(uint(id), updates); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	// Re-read and reschedule
	job, _ := database.GetCronJob(uint(id))
	if job != nil {
		if err := h.scheduler.UpdateJob(job); err != nil {
			respond(w, 400, J{"error": "invalid schedule: " + err.Error()})
			return
		}
	}

	h.hub.Broadcast(Event{Type: "cron:updated", Data: J{"id": id, "name": job.Name}})
	respond(w, 200, cronToJSON(job, h.scheduler))
}

func (h *Handlers) DeleteCron(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	h.scheduler.RemoveJob(uint(id))
	database.DeleteCronJob(uint(id))

	h.hub.Broadcast(Event{Type: "cron:deleted", Data: J{"id": id}})
	logAction("CRON_DELETED", fmt.Sprintf("%d", id), job.Name)
	respond(w, 200, J{"status": "deleted"})
}

func (h *Handlers) TriggerCron(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	if job.AgentProfileName != "" {
		if !checkProfilePerm(w, r, job.AgentProfileName, database.PermTriggerCrons) {
			return
		}
	}

	h.scheduler.TriggerJob(job)
	respond(w, 200, J{"status": "triggered", "id": id})
}

func (h *Handlers) ListCronExecutions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}
	if job.AgentProfileName != "" {
		if !checkProfilePerm(w, r, job.AgentProfileName, database.PermViewCrons) {
			return
		}
	}

	execs, err := database.ListCronExecutions(uint(id), 50)
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	out := make([]J, 0, len(execs))
	for _, e := range execs {
		j := J{
			"id":         e.ID,
			"cronJobId":  e.CronJobID,
			"agentId":    e.AgentID,
			"status":     e.Status,
			"durationMs": e.DurationMs,
			"ts":         e.CreatedAt.Format(time.RFC3339),
		}
		if e.Error != "" {
			j["error"] = e.Error
		}
		out = append(out, j)
	}
	respond(w, 200, out)
}

func (h *Handlers) AgentCreateCron(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Schedule    string  `json:"schedule"`
		RunAt       *string `json:"runAt"` // RFC3339 datetime for one-time execution
		Timezone    string  `json:"timezone"`
		Session     string  `json:"session"`
		Message     string  `json:"message"`
		AgentName   string  `json:"agentName"` // optional: caller self-identifies for multi-agent registrations
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Name == "" || req.Message == "" {
		respond(w, 400, J{"error": "name and message required"})
		return
	}
	if req.Schedule == "" && req.RunAt == nil {
		respond(w, 400, J{"error": "either schedule or runAt is required"})
		return
	}
	if req.Schedule != "" && req.RunAt != nil {
		respond(w, 400, J{"error": "cannot set both schedule and runAt"})
		return
	}

	var runAtTime *time.Time
	if req.RunAt != nil {
		t, err := time.Parse(time.RFC3339, *req.RunAt)
		if err != nil {
			respond(w, 400, J{"error": "runAt must be RFC3339 format, e.g. 2026-03-01T10:00:00+05:30"})
			return
		}
		runAtTime = &t
	}

	// Resolve profile: if caller passed agentName, use it directly (multi-agent case).
	// Otherwise fall back to first profile found for this registration (single-agent case).
	profileName := ""
	if req.AgentName != "" {
		if p, err := database.GetAgentProfile(req.AgentName); err == nil &&
			p.Registration != nil && *p.Registration == reg.Name {
			profileName = p.Name
		}
	}
	if profileName == "" {
		profiles, err := database.ListAgentProfiles()
		if err != nil {
			respond(w, 500, J{"error": "failed to resolve agent"})
			return
		}
		for _, p := range profiles {
			if p.Registration != nil && *p.Registration == reg.Name {
				profileName = p.Name
				break
			}
		}
	}
	if profileName == "" {
		respond(w, 400, J{"error": "no agent profile found for this registration"})
		return
	}

	tz := "UTC"
	if req.Timezone != "" {
		tz = req.Timezone
	}

	job := &database.CronJob{
		Name:             req.Name,
		Description:      req.Description,
		AgentProfileName: profileName,
		RegistrationName: reg.Name,
		Schedule:         req.Schedule,
		RunAt:            runAtTime,
		Timezone:         tz,
		Session:          req.Session,
		Message:          req.Message,
		Enabled:          true,
	}

	if err := database.CreateCronJob(job); err != nil {
		respond(w, 500, J{"error": "failed to create cron"})
		return
	}

	if err := h.scheduler.AddJob(job); err != nil {
		respond(w, 400, J{"error": "invalid schedule: " + err.Error()})
		database.DeleteCronJob(job.ID)
		return
	}

	h.hub.Broadcast(Event{Type: "cron:created", Data: J{"id": job.ID, "name": job.Name, "registrationName": reg.Name}})
	respond(w, 201, cronToJSON(job, h.scheduler))
}

func (h *Handlers) AgentListCrons(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)
	agentID := r.URL.Query().Get("agentId")

	jobs, err := database.ListCronJobsByRegistration(reg.Name)
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	// If agentId filter provided, only return jobs bound to that agent
	out := make([]J, 0, len(jobs))
	for i := range jobs {
		if agentID != "" && jobs[i].AgentProfileName != agentID {
			continue
		}
		out = append(out, cronToJSON(&jobs[i], h.scheduler))
	}
	respond(w, 200, out)
}

func (h *Handlers) AgentDeleteCron(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	// Verify the cron belongs to the caller's registration
	profile, err := database.GetAgentProfile(job.AgentProfileName)
	if err != nil || profile.Registration == nil || *profile.Registration != reg.Name {
		respond(w, 403, J{"error": "cron does not belong to your registration"})
		return
	}

	h.scheduler.RemoveJob(uint(id))
	database.DeleteCronJob(uint(id))

	logAction("CRON_DELETED", fmt.Sprintf("%d", id), job.Name)
	respond(w, 200, J{"status": "deleted"})
}

func (h *Handlers) AgentUpdateCron(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		respond(w, 400, J{"error": "invalid id"})
		return
	}

	job, err := database.GetCronJob(uint(id))
	if err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	// Verify the cron belongs to the caller's registration
	// Check direct registration_name first; fall back to agent profile membership (for older crons)
	if job.RegistrationName != "" && job.RegistrationName != reg.Name {
		respond(w, 403, J{"error": "cron does not belong to your registration"})
		return
	}
	if job.RegistrationName == "" {
		profile, err := database.GetAgentProfile(job.AgentProfileName)
		if err != nil || profile.Registration == nil || *profile.Registration != reg.Name {
			respond(w, 403, J{"error": "cron does not belong to your registration"})
			return
		}
	}

	// Agents may only update schedule/timing fields
	var req struct {
		Schedule *string `json:"schedule"`
		RunAt    *string `json:"runAt"` // RFC3339; send "" to clear
		Timezone *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	updates := map[string]any{}
	if req.Schedule != nil {
		updates["schedule"] = *req.Schedule
	}
	if req.RunAt != nil {
		if *req.RunAt == "" {
			updates["run_at"] = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.RunAt)
			if err != nil {
				respond(w, 400, J{"error": "runAt must be RFC3339 format, e.g. 2026-03-01T10:00:00+05:30"})
				return
			}
			updates["run_at"] = t
		}
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}

	// Backfill registrationName for old crons that were created before this field existed
	if job.RegistrationName == "" {
		updates["registration_name"] = reg.Name
	}

	if len(updates) == 0 {
		respond(w, 400, J{"error": "nothing to update"})
		return
	}

	if err := database.UpdateCronJob(uint(id), updates); err != nil {
		respond(w, 500, J{"error": "failed to update"})
		return
	}

	job, _ = database.GetCronJob(uint(id))
	if job != nil {
		if err := h.scheduler.UpdateJob(job); err != nil {
			respond(w, 400, J{"error": "invalid schedule: " + err.Error()})
			return
		}
	}

	respond(w, 200, cronToJSON(job, h.scheduler))
}

// --- Agent Profiles ---

func (h *Handlers) CreateAgentTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		RegistrationName string `json:"registrationName"`
		Image            string `json:"image"`
		MaxCount         int    `json:"maxCount"`
		TTLMinutes       int    `json:"ttlMinutes"`
		DeploymentConfig string `json:"deploymentConfig"`
		Source           string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respond(w, 400, J{"error": "name required"})
		return
	}

	if req.RegistrationName != "" {
		if _, err := database.GetRegistrationByName(req.RegistrationName); err != nil {
			respond(w, 404, J{"error": "registration not found: " + req.RegistrationName})
			return
		}
	}

	var regPtr *string
	if req.RegistrationName != "" {
		regPtr = &req.RegistrationName
	}
	profile, err := database.CreateAgentProfile(req.Name, req.Description, req.Image, req.DeploymentConfig, req.Source, regPtr, req.MaxCount, req.TTLMinutes)
	if err != nil {
		respond(w, 409, J{"error": "profile already exists or failed to create"})
		return
	}

	logAction("PROFILE_CREATED", profile.Name, "")
	h.hub.Broadcast(Event{Type: "profile:created", Data: J{"name": profile.Name}})
	respond(w, 201, agentProfileToJSON(profile))
}

func (h *Handlers) ListAgentTemplates(w http.ResponseWriter, r *http.Request) {
	profiles, err := database.ListAgentProfiles()
	if err != nil {
		respond(w, 500, J{"error": "failed to list"})
		return
	}

	u := userFromCtx(r)
	visibleProfiles, isAdmin := database.VisibleProfiles(u.ID)
	if !isAdmin {
		profileSet := make(map[string]bool, len(visibleProfiles))
		for _, p := range visibleProfiles {
			profileSet[p] = true
		}
		filtered := profiles[:0]
		for _, p := range profiles {
			if profileSet[p.Name] {
				filtered = append(filtered, p)
			}
		}
		profiles = filtered
	}

	out := make([]J, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, agentProfileToJSON(&p))
	}
	respond(w, 200, out)
}

func (h *Handlers) UpdateAgentTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		RegistrationName *string `json:"registrationName"`
		Image            *string `json:"image"`
		MaxCount         *int    `json:"maxCount"`
		TTLMinutes       *int    `json:"ttlMinutes"`
		DeploymentConfig *string `json:"deploymentConfig"`
		Source           *string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond(w, 400, J{"error": "invalid JSON"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.RegistrationName != nil {
		updates["registration_name"] = *req.RegistrationName
	}
	if req.Image != nil {
		updates["image"] = *req.Image
	}
	if req.MaxCount != nil {
		updates["max_count"] = *req.MaxCount
	}
	if req.TTLMinutes != nil {
		updates["ttl_minutes"] = *req.TTLMinutes
	}
	if req.DeploymentConfig != nil {
		updates["deployment_config"] = *req.DeploymentConfig
	}
	if req.Source != nil {
		updates["source"] = *req.Source
	}

	if len(updates) == 0 {
		respond(w, 400, J{"error": "nothing to update"})
		return
	}

	if err := database.UpdateAgentProfile(name, updates); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	logAction("PROFILE_UPDATED", name, fmt.Sprintf("%v", updates))
	h.hub.Broadcast(Event{Type: "profile:updated", Data: J{"name": name}})
	respond(w, 200, J{"status": "updated"})
}

func (h *Handlers) DeleteAgentTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	count, _ := database.CountAgentsByProfile(name)
	if count > 0 {
		respond(w, 409, J{"error": fmt.Sprintf("cannot delete — %d agent(s) are using this profile", count)})
		return
	}

	if err := database.DeleteAgentProfile(name); err != nil {
		respond(w, 404, J{"error": "not found"})
		return
	}

	logAction("PROFILE_DELETED", name, "")
	respond(w, 200, J{"status": "deleted"})
}

func agentProfileToJSON(p *database.AgentProfile) J {
	var deploymentConfig any
	json.Unmarshal([]byte(p.DeploymentConfig), &deploymentConfig)

	var regName any
	if p.Registration != nil {
		regName = *p.Registration
	}
	count, _ := database.CountAgentsByProfile(p.Name)
	return J{
		"name":             p.Name,
		"description":      p.Description,
		"registrationName": regName,
		"image":            p.Image,
		"maxCount":         p.MaxCount,
		"ttlMinutes":       p.TTLMinutes,
		"deploymentConfig": deploymentConfig,
		"source":           p.Source,
		"agents":           count,
		"createdAt":        p.CreatedAt.Format(time.RFC3339),
		"updatedAt":        p.UpdatedAt.Format(time.RFC3339),
	}
}

// --- Connections ---

func (h *Handlers) CreateConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source  string   `json:"source"`
		Target  string   `json:"target"`
		Targets []string `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Source == "" {
		respond(w, 400, J{"error": "source required"})
		return
	}

	// Support both single target and multiple targets
	targets := req.Targets
	if req.Target != "" {
		targets = append(targets, req.Target)
	}
	if len(targets) == 0 {
		respond(w, 400, J{"error": "at least one target required"})
		return
	}

	// Validate source agent exists
	srcAgents, _ := database.GetAgentsByName(req.Source)
	if len(srcAgents) == 0 {
		respond(w, 404, J{"error": "source agent not found: " + req.Source})
		return
	}

	var created []J
	for _, t := range targets {
		if t == req.Source {
			continue // skip self-connections
		}
		// Validate target agent exists
		tgtAgents, _ := database.GetAgentsByName(t)
		if len(tgtAgents) == 0 {
			continue
		}
		conn, err := database.CreateConnection(req.Source, t)
		if err != nil {
			continue // already exists, skip
		}
		auditData, _ := json.Marshal(J{"source": req.Source, "target": t})
		database.InsertAuditEvent("connection:created", string(auditData))
		h.hub.Broadcast(Event{Type: "connection:created", Data: J{"source": req.Source, "target": t}})
		created = append(created, J{"id": conn.ID, "source": conn.Source, "target": conn.Target, "createdAt": conn.CreatedAt.Format(time.RFC3339)})
	}

	respond(w, 201, created)
}

func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	source := q.Get("source")
	target := q.Get("target")

	var conns []database.Connection
	var err error
	if source != "" {
		conns, err = database.ListConnectionsFrom(source)
	} else if target != "" {
		conns, err = database.ListConnectionsTo(target)
	} else {
		conns, err = database.ListAllConnections()
	}
	if err != nil {
		respond(w, 500, J{"error": "failed to list connections"})
		return
	}

	out := make([]J, 0, len(conns))
	for _, c := range conns {
		out = append(out, J{"id": c.ID, "source": c.Source, "target": c.Target, "createdAt": c.CreatedAt.Format(time.RFC3339)})
	}
	respond(w, 200, out)
}

func (h *Handlers) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Source == "" || req.Target == "" {
		respond(w, 400, J{"error": "source and target required"})
		return
	}

	if err := database.DeleteConnection(req.Source, req.Target); err != nil {
		respond(w, 404, J{"error": "connection not found"})
		return
	}

	auditData, _ := json.Marshal(J{"source": req.Source, "target": req.Target})
	database.InsertAuditEvent("connection:deleted", string(auditData))
	h.hub.Broadcast(Event{Type: "connection:deleted", Data: J{"source": req.Source, "target": req.Target}})
	respond(w, 200, J{"status": "deleted"})
}

// --- Agent-to-Agent ---

func (h *Handlers) AgentChat(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)
	target := r.PathValue("target")

	// Check caller has connection to target (by agent name)
	// Get all agent names under caller's registration
	callerAgents, _ := database.ListAgents(reg.Name)
	hasConn := false
	for _, a := range callerAgents {
		if a.AgentProfile != nil && database.HasConnection(*a.AgentProfile, target) {
			hasConn = true
			break
		}
	}
	if !hasConn {
		respond(w, 403, J{"error": fmt.Sprintf("no agent under %q is connected to %q", reg.Name, target)})
		return
	}

	// Find a healthy agent by name
	targetAgents, err := database.GetAgentsByName(target)
	if err != nil || len(targetAgents) == 0 {
		respond(w, 404, J{"error": "no agent found with name: " + target})
		return
	}

	var targetAgent *database.Agent
	for _, a := range targetAgents {
		if a.Status == "healthy" {
			var meta map[string]any
			json.Unmarshal([]byte(a.Meta), &meta)
			if chatURL, _ := meta["chatUrl"].(string); chatURL != "" {
				targetAgent = &a
				break
			}
		}
	}
	if targetAgent == nil {
		respond(w, 503, J{"error": "no healthy agent available for " + target})
		return
	}

	// Parse chatUrl from target agent
	var meta map[string]any
	json.Unmarshal([]byte(targetAgent.Meta), &meta)
	chatURL, _ := meta["chatUrl"].(string)

	// Read and proxy the request body
	var req struct {
		Message string `json:"message"`
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		respond(w, 400, J{"error": "message required"})
		return
	}

	// Derive session: pass caller's session prefixed with caller name for traceability
	session := req.Session
	if session == "" {
		session = "delegate:" + reg.Name
	} else {
		session = "delegate:" + reg.Name + ":" + session
	}

	payload := J{"message": req.Message, "session": session}
	body, _ := json.Marshal(payload)

	proxyReq, err := http.NewRequestWithContext(context.Background(), "POST", chatURL, bytes.NewReader(body))
	if err != nil {
		respond(w, 500, J{"error": "failed to create request"})
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Accept", "text/event-stream, application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+targetAgent.Token)

	resp, err := chatClient.Do(proxyReq)
	if err != nil {
		respond(w, 502, J{"error": "target agent unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	flusher, canFlush := w.(http.Flusher)

	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(resp.StatusCode)

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

func (h *Handlers) AgentConnections(w http.ResponseWriter, r *http.Request) {
	reg := registrationFromCtx(r)

	// Get all agent names under this registration
	regAgents, _ := database.ListAgents(reg.Name)

	// If ?agent=<profile> is provided, return a flat array of just that agent's connections.
	// Otherwise return a grouped map { "cto": [...], "mflead": [...] } so multi-agent
	// containers can self-identify using their own key.
	agentFilter := r.URL.Query().Get("agent")

	buildEntries := func(profile string) []J {
		conns, _ := database.ListConnectionsFrom(profile)
		entries := make([]J, 0, len(conns))
		for _, c := range conns {
			entry := J{"name": c.Target}
			targets, _ := database.GetAgentsByName(c.Target)
			var agentList []J
			for _, t := range targets {
				if t.Status != "healthy" {
					continue
				}
				profileName := ""
				if t.AgentProfile != nil {
					profileName = *t.AgentProfile
				}
				ae := J{"id": t.ID, "name": profileName, "status": t.Status}
				var meta map[string]any
				json.Unmarshal([]byte(t.Meta), &meta)
				if runner, _ := meta["runner"].(string); runner != "" {
					ae["runner"] = runner
				}
				agentList = append(agentList, ae)
			}
			entry["agents"] = agentList
			entries = append(entries, entry)
		}
		return entries
	}

	if agentFilter != "" {
		respond(w, 200, buildEntries(agentFilter))
		return
	}

	out := map[string][]J{}
	for _, a := range regAgents {
		if a.AgentProfile == nil {
			continue
		}
		out[*a.AgentProfile] = buildEntries(*a.AgentProfile)
	}
	respond(w, 200, out)
}

// --- Helpers ---

func agentProfileName(a *database.Agent) string {
	if a.AgentProfile != nil {
		return *a.AgentProfile
	}
	return ""
}

func agentToJSON(a *database.Agent) J {
	j := J{
		"id":           a.ID,
		"name":         agentProfileName(a),
		"registration": database.GetAgentRegistrationName(a),
		"status":       a.Status,
		"stats": J{
			"allowed": a.StatsAllowed, "blocked": a.StatsBlocked,
			"avgMs": a.StatsAvgMs, "minMs": a.StatsMinMs, "maxMs": a.StatsMaxMs,
			"reqCount": a.StatsReqCount,
		},
		"registeredAt":  a.RegisteredAt.Format(time.RFC3339),
		"lastHeartbeat": a.LastHeartbeat.Format(time.RFC3339),
	}
	if a.KillReason != "" {
		j["killReason"] = a.KillReason
	}

	var env, meta, gw json.RawMessage
	json.Unmarshal([]byte(a.Environment), &env)
	json.Unmarshal([]byte(a.Meta), &meta)
	json.Unmarshal([]byte(a.Gateway), &gw)
	j["environment"] = env
	j["meta"] = meta
	j["gateway"] = gw

	return j
}

func cronToJSON(job *database.CronJob, sched CronScheduler) J {
	j := J{
		"id":               job.ID,
		"name":             job.Name,
		"description":      job.Description,
		"agentName":        job.AgentProfileName,
		"registrationName": job.RegistrationName,
		"schedule": job.Schedule,
		"timezone":         job.Timezone,
		"session":          job.Session,
		"message":          job.Message,
		"enabled":          job.Enabled,
		"lastStatus":       job.LastStatus,
		"createdAt":        job.CreatedAt.Format(time.RFC3339),
		"updatedAt":        job.UpdatedAt.Format(time.RFC3339),
	}
	if job.RunAt != nil {
		j["runAt"] = job.RunAt.Format(time.RFC3339)
	}
	if job.LastRunAt != nil {
		j["lastRunAt"] = job.LastRunAt.Format(time.RFC3339)
	}
	if job.LastError != "" {
		j["lastError"] = job.LastError
	}
	// Get live next run from scheduler (more accurate than DB)
	if next := sched.NextRunTime(job.ID); next != nil {
		j["nextRunAt"] = next.Format(time.RFC3339)
	} else if job.NextRunAt != nil {
		j["nextRunAt"] = job.NextRunAt.Format(time.RFC3339)
	}
	return j
}

func genToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "rt_" + hex.EncodeToString(b)
}

func genAgentToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "ag_" + hex.EncodeToString(b)
}

func logAction(action, id, detail string) {
	entry := J{"ts": time.Now().UTC().UTC().Format(time.RFC3339), "action": action, "id": id}
	if detail != "" {
		entry["detail"] = detail
	}
	b, _ := json.Marshal(entry)
	log.Println(string(b))
}

func respond(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
