package api

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"control-plane/internal/database"
	"control-plane/internal/metrics"
	"control-plane/internal/storage"
	"control-plane/internal/ui"
)

func NewRouter(hub *Hub, scheduler CronScheduler, oidc *OIDCConfig, store storage.Store, publicBaseURL, signSecret string) http.Handler {
	h := NewHandlers(hub, scheduler, oidc, store, publicBaseURL, signSecret)

	mux := http.NewServeMux()

	// UI (embedded HTML + static assets)
	mux.HandleFunc("GET /", ui.Handler())
	mux.Handle("GET /ui/", ui.StaticHandler())

	// SSE — JWT via query param (EventSource can't set headers)
	mux.HandleFunc("GET /events", hub.ServeHTTP())

	// Public
	mux.HandleFunc("GET /llms.txt", h.LLMsTxt)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /.well-known/agent-card.json", h.AgentCard)
	mux.HandleFunc("GET /a2a/{agent}/agent-card.json", h.AgentCard)

	// Auth
	mux.HandleFunc("POST /auth/login", h.Login)
	mux.HandleFunc("GET /auth/me", h.withAuth(h.Me))

	// OIDC — public, no auth required
	mux.HandleFunc("GET /auth/oidc/config", h.OIDCProviderConfig)
	mux.HandleFunc("GET /auth/oidc/start", h.OIDCStart)
	mux.HandleFunc("GET /auth/oidc/callback", h.OIDCCallback)

	// Users
	mux.HandleFunc("GET /users", h.withPerm(database.PermManageUsers, h.ListUsers))
	mux.HandleFunc("POST /users", h.withPerm(database.PermManageUsers, h.CreateUser))
	mux.HandleFunc("GET /users/{id}", h.withPerm(database.PermManageUsers, h.GetUser))
	mux.HandleFunc("PUT /users/{id}", h.withPerm(database.PermManageUsers, h.UpdateUser))
	mux.HandleFunc("DELETE /users/{id}", h.withPerm(database.PermManageUsers, h.DeleteUser))
	mux.HandleFunc("GET /users/{id}/access", h.withPerm(database.PermManageProfiles, h.UserAccess))
	mux.HandleFunc("GET /users/{id}/identities", h.withPerm(database.PermManageUsers, h.ListUserIdentities))
	mux.HandleFunc("POST /users/{id}/identities", h.withPerm(database.PermManageUsers, h.LinkUserIdentity))
	mux.HandleFunc("DELETE /users/{id}/identities/{provider}", h.withPerm(database.PermManageUsers, h.UnlinkUserIdentity))

	// Roles
	mux.HandleFunc("GET /roles", h.withAuth(h.ListRoles))
	mux.HandleFunc("POST /roles", h.withPerm(database.PermManageRoles, h.CreateRole))
	mux.HandleFunc("GET /roles/{id}", h.withAuth(h.GetRole))
	mux.HandleFunc("PUT /roles/{id}", h.withPerm(database.PermManageRoles, h.UpdateRole))
	mux.HandleFunc("DELETE /roles/{id}", h.withPerm(database.PermManageRoles, h.DeleteRole))
	mux.HandleFunc("POST /roles/{id}/permissions", h.withPerm(database.PermManageRoles, h.AddRolePermission))
	mux.HandleFunc("DELETE /roles/{id}/permissions/{perm}", h.withPerm(database.PermManageRoles, h.RemoveRolePermission))

	// Profile ACL
	mux.HandleFunc("GET /agent-profiles/{name}/acl", h.withPerm(database.PermManageProfiles, h.ListProfileACL))
	mux.HandleFunc("POST /agent-profiles/{name}/acl", h.withPerm(database.PermManageProfiles, h.SetProfileACL))
	mux.HandleFunc("DELETE /agent-profiles/{name}/acl/{pid}", h.withPerm(database.PermManageProfiles, h.DeleteProfileACL))
	mux.HandleFunc("DELETE /agent-profiles/{name}/acl/{ptype}/{pid}", h.withPerm(database.PermManageProfiles, h.DeleteProfileACL))

	// Agent ACL (per-individual-agent grants)
	mux.HandleFunc("GET /agents/{id}/acl", h.withPerm(database.PermManageProfiles, h.ListAgentACL))
	mux.HandleFunc("POST /agents/{id}/acl", h.withPerm(database.PermManageProfiles, h.SetAgentACL))
	mux.HandleFunc("DELETE /agents/{id}/acl/{pid}", h.withPerm(database.PermManageProfiles, h.DeleteAgentACL))
	mux.HandleFunc("DELETE /agents/{id}/acl/{ptype}/{pid}", h.withPerm(database.PermManageProfiles, h.DeleteAgentACL))

	// Human groups (teams)
	mux.HandleFunc("GET /groups", h.withPerm(database.PermManageProfiles, h.ListGroups))
	mux.HandleFunc("POST /groups", h.withPerm(database.PermManageProfiles, h.CreateGroup))
	mux.HandleFunc("PUT /groups/{id}", h.withPerm(database.PermManageProfiles, h.UpdateGroup))
	mux.HandleFunc("DELETE /groups/{id}", h.withPerm(database.PermManageProfiles, h.DeleteGroup))
	mux.HandleFunc("GET /groups/{id}/access", h.withPerm(database.PermManageProfiles, h.GroupAccess))
	mux.HandleFunc("GET /groups/{id}/members", h.withPerm(database.PermManageProfiles, h.ListGroupMembers))
	mux.HandleFunc("POST /groups/{id}/members", h.withPerm(database.PermManageProfiles, h.AddGroupMember))
	mux.HandleFunc("DELETE /groups/{id}/members/{user_id}", h.withPerm(database.PermManageProfiles, h.RemoveGroupMember))

	// Registrations
	mux.HandleFunc("POST /agent-registrations", h.withPerm(database.PermManageRegistrations, h.CreateRegistration))
	mux.HandleFunc("GET /agent-registrations", h.withPermOrEmpty(database.PermManageRegistrations, h.ListRegistrations))
	mux.HandleFunc("PUT /agent-registrations/{name}", h.withPerm(database.PermManageRegistrations, h.UpdateRegistration))
	mux.HandleFunc("PUT /agent-registrations/{name}/archive", h.withPerm(database.PermManageRegistrations, h.ArchiveRegistration))
	mux.HandleFunc("DELETE /agent-registrations/{name}", h.withPerm(database.PermManageRegistrations, h.DeleteRegistration))

	// Agent Profiles
	mux.HandleFunc("POST /agent-profiles", h.withPerm(database.PermManageProfiles, h.CreateAgentTemplate))
	mux.HandleFunc("GET /agent-profiles", h.withAuth(h.ListAgentTemplates))
	mux.HandleFunc("PUT /agent-profiles/{name}", h.withPerm(database.PermManageProfiles, h.UpdateAgentTemplate))
	mux.HandleFunc("DELETE /agent-profiles/{name}", h.withPerm(database.PermManageProfiles, h.DeleteAgentTemplate))

	// Connections
	mux.HandleFunc("POST /connections", h.withPerm(database.PermManageConnections, h.CreateConnection))
	mux.HandleFunc("GET /connections", h.withPerm(database.PermManageConnections, h.ListConnections))
	mux.HandleFunc("DELETE /connections", h.withPerm(database.PermManageConnections, h.DeleteConnection))

	// Sidecar
	mux.HandleFunc("POST /register", h.withAgent(h.Register))
	mux.HandleFunc("POST /heartbeat", h.withAgent(h.Heartbeat))
	mux.HandleFunc("POST /logs", h.withAgent(h.IngestLogs))
	mux.HandleFunc("POST /monitor/heartbeat", h.withAgent(h.MonitorHeartbeat))
	mux.HandleFunc("DELETE /register/{id}", h.withAgent(h.Deregister))
	mux.HandleFunc("GET /config", h.withAgent(h.Config))

	// Crons - Admin
	mux.HandleFunc("POST /crons", h.withPerm(database.PermManageCrons, h.CreateCron))
	mux.HandleFunc("GET /crons", h.withAuth(h.ListCrons))
	mux.HandleFunc("GET /crons/{id}", h.withAuth(h.GetCron))
	mux.HandleFunc("PUT /crons/{id}", h.withPerm(database.PermManageCrons, h.UpdateCron))
	mux.HandleFunc("DELETE /crons/{id}", h.withPerm(database.PermManageCrons, h.DeleteCron))
	mux.HandleFunc("POST /crons/{id}/trigger", h.withAuth(h.TriggerCron))
	mux.HandleFunc("GET /crons/{id}/executions", h.withAuth(h.ListCronExecutions))

	// Self-service (developer CLI): PATs + agent discovery
	mux.HandleFunc("GET /me/agents", h.withAuth(h.MyAgents))
	mux.HandleFunc("GET /me/tokens", h.withAuth(h.ListMyTokens))
	mux.HandleFunc("POST /me/tokens", h.withAuth(h.CreateMyToken))
	mux.HandleFunc("DELETE /me/tokens/{id}", h.withAuth(h.DeleteMyToken))

	// Attachments / uploads
	mux.HandleFunc("POST /uploads", h.withAuth(h.CreateUpload))
	mux.HandleFunc("GET /uploads", h.withAuth(h.ListUploads))
	mux.HandleFunc("GET /uploads/{id}", h.GetUpload) // auth via owner token OR signed query token
	mux.HandleFunc("DELETE /uploads/{id}", h.withAuth(h.DeleteUpload))

	// Agent-to-Agent
	mux.HandleFunc("POST /a2a/{agent}", h.A2A)
	mux.HandleFunc("GET /tasks", h.ListTasks) // auth handled inside (user/PAT or agent/registration)
	mux.HandleFunc("GET /tasks/{id}", h.GetTask)
	mux.HandleFunc("GET /tasks/{id}/trace", h.GetTaskTrace)
	mux.HandleFunc("GET /tasks/{id}/thread", h.GetTaskThread)
	mux.HandleFunc("POST /agent-activity", h.withAgent(h.IngestAgentActivity)) // clutch reports local hops
	mux.HandleFunc("POST /agent-chat/{target}", h.withAgent(h.AgentChat))
	mux.HandleFunc("GET /agent-connections", h.withAgent(h.AgentConnections))

	// Crons - Sidecar
	mux.HandleFunc("POST /agent-crons", h.withAgent(h.AgentCreateCron))
	mux.HandleFunc("GET /agent-crons", h.withAgent(h.AgentListCrons))
	mux.HandleFunc("PUT /agent-crons/{id}", h.withAgent(h.AgentUpdateCron))
	mux.HandleFunc("DELETE /agent-crons/{id}", h.withAgent(h.AgentDeleteCron))

	// Agents & Query
	mux.HandleFunc("GET /agents", h.withAuth(h.ListAgents))
	mux.HandleFunc("GET /agents/{id}", h.withAuth(h.GetAgent))
	mux.HandleFunc("POST /agents/{id}/chat", h.withAuth(h.ChatProxy))
	mux.HandleFunc("GET /agents/{id}/workspace/locks", h.withAuth(h.WorkspaceLocksProxy))
	mux.HandleFunc("PUT /agents/{id}/workspace/locks", h.withAuth(h.WorkspaceLocksProxy))
	mux.HandleFunc("GET /agents/{id}/workspace", h.withAuth(h.WorkspaceProxy))
	mux.HandleFunc("GET /agents/{id}/sessions", h.withAuth(h.SessionsProxy))
	mux.HandleFunc("GET /api/metrics", h.withPerm(database.PermViewMetrics, h.GetMetrics))
	mux.HandleFunc("GET /api/metrics/series", h.withPerm(database.PermViewMetrics, h.GetMetricsSeries))
	mux.HandleFunc("GET /logs", h.withPerm(database.PermViewLogs, h.QueryLogs))
	mux.HandleFunc("GET /logs/stats", h.withPerm(database.PermViewLogs, h.LogStats))
	mux.HandleFunc("GET /audit", h.withPerm(database.PermViewAudit, h.QueryAudit))

	// Prometheus scrape endpoint
	mux.Handle("GET /metrics", metrics.Handler())

	return metrics.Middleware(otelhttp.NewHandler(mux, "control-plane"))
}
