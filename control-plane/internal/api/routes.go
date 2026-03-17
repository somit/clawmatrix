package api

import (
	"net/http"

	"control-plane/internal/ui"
)

func NewRouter(adminToken string, hub *Hub, scheduler CronScheduler) http.Handler {
	h := NewHandlers(adminToken, hub, scheduler)

	mux := http.NewServeMux()

	// UI (embedded HTML + static assets)
	mux.HandleFunc("GET /", ui.Handler())
	mux.Handle("GET /ui/", ui.StaticHandler())

	// SSE
	mux.HandleFunc("GET /events", hub.ServeHTTP(adminToken))

	// LLMs
	mux.HandleFunc("GET /llms.txt", h.LLMsTxt)

	// Admin
	mux.HandleFunc("POST /agent-registrations", h.withAdmin(h.CreateRegistration))
	mux.HandleFunc("GET /agent-registrations", h.withAdmin(h.ListRegistrations))
	mux.HandleFunc("PUT /agent-registrations/{name}", h.withAdmin(h.UpdateRegistration))
	mux.HandleFunc("PUT /agent-registrations/{name}/archive", h.withAdmin(h.ArchiveRegistration))
	mux.HandleFunc("DELETE /agent-registrations/{name}", h.withAdmin(h.DeleteRegistration))

	// Agent Profiles (admin)
	mux.HandleFunc("POST /agent-profiles", h.withAdmin(h.CreateAgentTemplate))
	mux.HandleFunc("GET /agent-profiles", h.withAdmin(h.ListAgentTemplates))
	mux.HandleFunc("PUT /agent-profiles/{name}", h.withAdmin(h.UpdateAgentTemplate))
	mux.HandleFunc("DELETE /agent-profiles/{name}", h.withAdmin(h.DeleteAgentTemplate))

	// Connections (admin)
	mux.HandleFunc("POST /connections", h.withAdmin(h.CreateConnection))
	mux.HandleFunc("GET /connections", h.withAdmin(h.ListConnections))
	mux.HandleFunc("DELETE /connections", h.withAdmin(h.DeleteConnection))

	// Sidecar
	mux.HandleFunc("POST /register", h.withAgent(h.Register))
	mux.HandleFunc("POST /heartbeat", h.withAgent(h.Heartbeat))
	mux.HandleFunc("POST /logs", h.withAgent(h.IngestLogs))
	mux.HandleFunc("POST /monitor/heartbeat", h.withAgent(h.MonitorHeartbeat))
	mux.HandleFunc("DELETE /register/{id}", h.withAgent(h.Deregister))
	mux.HandleFunc("GET /config", h.withAgent(h.Config))

	// Crons - Admin
	mux.HandleFunc("POST /crons", h.withAdmin(h.CreateCron))
	mux.HandleFunc("GET /crons", h.withAdmin(h.ListCrons))
	mux.HandleFunc("GET /crons/{id}", h.withAdmin(h.GetCron))
	mux.HandleFunc("PUT /crons/{id}", h.withAdmin(h.UpdateCron))
	mux.HandleFunc("DELETE /crons/{id}", h.withAdmin(h.DeleteCron))
	mux.HandleFunc("POST /crons/{id}/trigger", h.withAdmin(h.TriggerCron))
	mux.HandleFunc("GET /crons/{id}/executions", h.withAdmin(h.ListCronExecutions))

	// Agent-to-Agent
	mux.HandleFunc("POST /agent-chat/{target}", h.withAgent(h.AgentChat))
	mux.HandleFunc("GET /agent-connections", h.withAgent(h.AgentConnections))

	// Crons - Sidecar
	mux.HandleFunc("POST /agent-crons", h.withAgent(h.AgentCreateCron))
	mux.HandleFunc("GET /agent-crons", h.withAgent(h.AgentListCrons))
	mux.HandleFunc("PUT /agent-crons/{id}", h.withAgent(h.AgentUpdateCron))
	mux.HandleFunc("DELETE /agent-crons/{id}", h.withAgent(h.AgentDeleteCron))

	// Query
	mux.HandleFunc("GET /agents", h.withAdmin(h.ListAgents))
	mux.HandleFunc("GET /agents/{id}", h.withAdmin(h.GetAgent))
	mux.HandleFunc("POST /agents/{id}/chat", h.withAdmin(h.ChatProxy))
	mux.HandleFunc("GET /agents/{id}/workspace/locks", h.withAdmin(h.WorkspaceLocksProxy))
	mux.HandleFunc("PUT /agents/{id}/workspace/locks", h.withAdmin(h.WorkspaceLocksProxy))
	mux.HandleFunc("GET /agents/{id}/workspace", h.withAdmin(h.WorkspaceProxy))
	mux.HandleFunc("GET /agents/{id}/sessions", h.withAdmin(h.SessionsProxy))
	mux.HandleFunc("GET /metrics", h.withAdmin(h.GetMetrics))
	mux.HandleFunc("GET /metrics/series", h.withAdmin(h.GetMetricsSeries))
	mux.HandleFunc("GET /logs", h.withAdmin(h.QueryLogs))
	mux.HandleFunc("GET /logs/stats", h.withAdmin(h.LogStats))
	mux.HandleFunc("GET /audit", h.withAdmin(h.QueryAudit))
	mux.HandleFunc("GET /health", h.Health)

	return mux
}
