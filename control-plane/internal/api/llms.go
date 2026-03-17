package api

import (
	"fmt"
	"net/http"
)

const llmsTxtTemplate = `# ClawMatrix — API Reference

> This document is designed for LLMs (like Claude) to understand and call the API.

## Base URL

%s

## Authentication

All endpoints except GET /health and GET /llms.txt require an admin token.

Send it as a Bearer token:
  Authorization: Bearer <ADMIN_TOKEN>

**IMPORTANT:** You do not have the admin token yet. Before making any API call, ask the user:
"What is your admin token for the control plane?"

Once they provide it, use it for all subsequent requests.

---

## Endpoints

### Registrations

Registrations define agent types. Agents register against a registration.

#### POST /agent-registrations
Create a new registration.

Request body:
  {
    "name": "string (required, unique)",
    "description": "string",
    "allowlist": ["domain1.com", "domain2.com"],
    "labels": {"key": "value"},
    "maxRunners": 0,
    "ttlMinutes": 0
  }

Note: Connections between registrations are managed via the /connections API (see below).

Response 201:
  {"name": "string", "token": "rt_..."}

The returned token is used by agents (sidecars) to register against this registration.

Example:
  curl -X POST %s/agent-registrations \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"name":"my-agent","description":"My agent registration","maxRunners":1}'

#### GET /agent-registrations
List all registrations.

Response 200:
  [
    {
      "name": "string",
      "description": "string",
      "allowlist": ["string"],
      "labels": {"key": "value"},
      "maxRunners": 0,
      "ttlMinutes": 0,
      "agents": 0,
      "totalRegistered": 0,
      "archived": false,
      "updatedAt": "RFC3339"
    }
  ]

Example:
  curl %s/agent-registrations \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### PUT /agent-registrations/{name}
Update a registration. All fields are optional — only provided fields are updated.

Request body:
  {
    "name": "string (new name, must be unique)",
    "description": "string",
    "allowlist": ["string"],
    "labels": {"key": "value"},
    "maxRunners": 0,
    "ttlMinutes": 0
  }

Response 200: {"status": "updated"}

Example:
  curl -X PUT %s/agent-registrations/my-agent \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"description":"Updated description","maxRunners":2}'

#### PUT /agent-registrations/{name}/archive
Archive or unarchive a registration.

Request body:
  {"archived": true}

Response 200: {"status": "ok", "archived": true}

Example:
  curl -X PUT %s/agent-registrations/my-agent/archive \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"archived":true}'

#### DELETE /agent-registrations/{name}
Delete a registration. Fails if any agents have ever registered (use archive instead).

Response 200: {"status": "deleted"}
Response 409: {"error": "cannot delete — agents have registered with this registration. Use archive instead."}

Example:
  curl -X DELETE %s/agent-registrations/my-agent \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

---

### Connections

Connections define a directed graph between registrations. "A can talk to B" = Connection{source:"a", target:"b"}.
Connections are directed: A->B does not imply B->A.

#### POST /connections
Create a connection between two registrations.

Request body:
  {
    "source": "string (required, registration name)",
    "target": "string (required, registration name)"
  }

Response 201:
  {"id": 1, "source": "ceo", "target": "cto", "createdAt": "RFC3339"}

Example:
  curl -X POST %s/connections \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"source":"ceo","target":"cto"}'

#### GET /connections
List connections. Optional filters by source or target.

Query params:
  source — filter by source registration name (optional)
  target — filter by target registration name (optional)

Response 200:
  [{"id": 1, "source": "ceo", "target": "cto", "createdAt": "RFC3339"}]

Example:
  curl "%s/connections?source=ceo" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### DELETE /connections
Delete a connection.

Request body:
  {"source": "ceo", "target": "cto"}

Response 200: {"status": "deleted"}

Example:
  curl -X DELETE %s/connections \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"source":"ceo","target":"cto"}'

---

### Agents

Agents are running instances registered against a registration.

#### GET /agents
List agents. Optional filter by registration type.

Query params:
  type — registration name filter (optional)

Response 200:
  [
    {
      "id": "registration-name-instanceid",
      "registration": "string",
      "status": "healthy|stale|kill",
      "stats": {
        "allowed": 0, "blocked": 0,
        "avgMs": 0, "minMs": 0, "maxMs": 0,
        "reqCount": 0
      },
      "registeredAt": "RFC3339",
      "lastHeartbeat": "RFC3339",
      "environment": {},
      "meta": {},
      "gateway": {}
    }
  ]

Example:
  curl "%s/agents?type=my-agent" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /agents/{id}
Get a single agent's details.

Response 200: Same shape as one element of the list above.

Example:
  curl %s/agents/my-agent-abc123 \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### POST /agents/{id}/chat
Send a chat message to an agent. The agent must have a chatUrl in its meta.

Request body:
  {
    "message": "string (required)",
    "session": "string (optional, for session continuity)"
  }

Response: Streams the agent's reply (may be SSE or JSON depending on agent).

Example:
  curl -X POST %s/agents/my-agent-abc123/chat \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"message":"Hello, what can you do?"}'

#### GET /agents/{id}/workspace
Browse an agent's workspace files. The agent must have a workspaceUrl in its meta.

Query params:
  path — file or directory path within the workspace (optional)

Response: File listing or file content from the agent's workspace.

Example:
  curl "%s/agents/my-agent-abc123/workspace?path=/" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /agents/{id}/workspace/locks
List locked workspace files. Returns a JSON array of locked file paths.

Response 200: ["SOUL.md", "TOOLS.md"]

Example:
  curl %s/agents/my-agent-abc123/workspace/locks \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### PUT /agents/{id}/workspace/locks
Lock or unlock a workspace file. Locking sets chmod 444 (read-only), unlocking restores
chmod 644 (writable). This is hard filesystem enforcement — the agent gets "permission
denied" at the OS level if it tries to write a locked file.

Request body:
  {"path": "SOUL.md", "locked": true}

Response 200: Updated lock list, e.g. ["SOUL.md", "TOOLS.md"]

Example — lock a file:
  curl -X PUT %s/agents/my-agent-abc123/workspace/locks \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"path":"SOUL.md","locked":true}'

Example — unlock a file:
  curl -X PUT %s/agents/my-agent-abc123/workspace/locks \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"path":"SOUL.md","locked":false}'

#### GET /agents/{id}/sessions
List or read agent chat sessions. The agent must have a sessionsUrl in its meta.

Query params:
  name — session name filter (optional)

Response: List of sessions or session content.

Example:
  curl "%s/agents/my-agent-abc123/sessions" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

---

### Crons

Scheduled tasks that send messages to agents on a cron schedule or at a specific time.

#### POST /crons
Create a cron job.

Request body:
  {
    "name": "string (required)",
    "registrationName": "string (required, must exist)",
    "message": "string (required, the message to send)",
    "schedule": "cron expression (e.g. '0 9 * * *')",
    "runAt": "RFC3339 datetime for one-time execution (e.g. '2026-03-01T10:00:00+05:30')",
    "timezone": "string (default: UTC, e.g. 'Asia/Kolkata')",
    "session": "string (optional, for session continuity)",
    "description": "string (optional)",
    "agentId": "string (optional, pin to specific agent)",
    "enabled": true
  }

Exactly one of schedule or runAt is required (not both).

Response 201:
  {
    "id": 1,
    "name": "string",
    "description": "string",
    "agentId": "string",
    "registrationName": "string",
    "schedule": "string",
    "runAt": "RFC3339 (if set)",
    "timezone": "string",
    "session": "string",
    "message": "string",
    "enabled": true,
    "lastStatus": "string",
    "nextRunAt": "RFC3339",
    "createdAt": "RFC3339",
    "updatedAt": "RFC3339"
  }

Example:
  curl -X POST %s/crons \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"name":"daily-report","registrationName":"my-agent","schedule":"0 9 * * *","timezone":"Asia/Kolkata","message":"Generate the daily report"}'

#### GET /crons
List cron jobs. Optional filter by registration type.

Query params:
  type — registration name filter (optional)

Response 200: Array of cron objects (same shape as POST response).

Example:
  curl "%s/crons?type=my-agent" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /crons/{id}
Get a single cron job's details.

Response 200: Cron object.

Example:
  curl %s/crons/1 \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### PUT /crons/{id}
Update a cron job. All fields are optional — only provided fields are updated.

Request body:
  {
    "name": "string",
    "description": "string",
    "agentId": "string",
    "schedule": "string",
    "runAt": "RFC3339 or empty string to clear",
    "timezone": "string",
    "session": "string",
    "message": "string",
    "enabled": true
  }

Response 200: Updated cron object.

Example:
  curl -X PUT %s/crons/1 \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"schedule":"0 10 * * *","message":"Updated message"}'

#### DELETE /crons/{id}
Delete a cron job.

Response 200: {"status": "deleted"}

Example:
  curl -X DELETE %s/crons/1 \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### POST /crons/{id}/trigger
Trigger a cron job immediately (runs it now regardless of schedule).

Response 200: {"status": "triggered", "id": 1}

Example:
  curl -X POST %s/crons/1/trigger \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /crons/{id}/executions
List recent executions of a cron job (up to 50).

Response 200:
  [
    {
      "id": 1,
      "cronJobId": 1,
      "agentId": "string",
      "status": "success|failed|timeout",
      "durationMs": 0,
      "error": "string (if failed)",
      "ts": "RFC3339"
    }
  ]

Example:
  curl %s/crons/1/executions \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

---

### Monitoring

#### GET /metrics
Query raw heartbeat metrics.

Query params:
  type   — registration name filter (optional)
  agent  — agent ID filter (optional)
  since  — duration like "1h", "30m" or RFC3339 timestamp (default: 1h)

Response 200:
  [
    {
      "agentId": "string",
      "registration": "string",
      "allowed": 0, "blocked": 0,
      "avgMs": 0, "minMs": 0, "maxMs": 0,
      "reqCount": 0,
      "ts": "RFC3339"
    }
  ]

Example:
  curl "%s/metrics?type=my-agent&since=1h" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /metrics/series
Get time-bucketed metric series for charting.

Query params:
  type   — registration name filter (optional)
  agent  — agent ID filter (optional)
  bucket — bucket duration like "5m", "1h" (default: 5m)
  since  — duration or RFC3339 (default: 1h)

Response 200:
  [
    {
      "ts": "RFC3339",
      "allowed": 0,
      "blocked": 0,
      "avgMs": 0
    }
  ]

Example:
  curl "%s/metrics/series?type=my-agent&bucket=5m&since=1h" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /logs
Query request logs from agents.

Query params:
  type   — registration name filter (optional)
  agent  — agent ID filter (optional)
  action — "allowed" or "blocked" (optional)
  domain — domain filter (optional)
  since  — duration or RFC3339 (default: 1h)

Response 200:
  [
    {
      "agentId": "string",
      "registration": "string",
      "domain": "string",
      "method": "GET|POST|...",
      "path": "string",
      "action": "allowed|blocked",
      "status": 200,
      "latencyMs": 0,
      "ts": "RFC3339"
    }
  ]

Example:
  curl "%s/logs?action=blocked&since=30m" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

#### GET /audit
Query audit events (admin actions, registrations, etc.).

Query params:
  since — duration like "24h" (default: 24h)

Response 200:
  [
    {
      "type": "string",
      "data": {},
      "ts": "RFC3339"
    }
  ]

Example:
  curl "%s/audit?since=24h" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

---

### Other

#### GET /health
Health check. No auth required.

Response 200:
  {"status": "ok", "agents": 0, "healthy": 0, "stale": 0}

Example:
  curl %s/health

#### GET /events
Server-Sent Events stream for real-time updates. Requires admin auth.

Events include: registration:created, registration:updated, agent:registered, agent:killed,
agent:recovered, cron:created, cron:updated, cron:deleted, log:batch.

Example:
  curl -N %s/events \
    -H "Authorization: Bearer <ADMIN_TOKEN>"

---

## Common Workflows

### Initial Setup (after DB reset)
1. Create a registration: POST /agent-registrations
2. Note the returned token — agents need it to register
3. Start an agent with that token — it auto-registers via POST /register
4. Verify: GET /agents to see the registered agent

### Schedule a Task
1. GET /agent-registrations to find the registration name
2. GET /agents?type=<registration> to find a running agent
3. POST /crons with the registration name, schedule, and message

### Monitor an Agent
1. GET /agents/{id} for current status and stats
2. GET /metrics?agent={id}&since=1h for recent metrics
3. GET /logs?agent={id}&since=30m for recent request logs

### Chat with an Agent
1. GET /agents to find an agent with status "healthy"
2. POST /agents/{id}/chat with your message
3. Use the same session value for follow-up messages to maintain context

### Edit an Agent's Workspace Files (Workspace Editor Chat)
Use the special session "autobot-manager-workspace-editor-chat" to instruct an agent
to edit files in its own workspace. The server prefixes messages on this session with
[workspace-editor] so the agent knows it should make file changes.

1. Find the agent: GET /agents?type=<registration> — pick a healthy agent
2. Browse its current files: GET /agents/{id}/workspace (root listing)
3. Read a specific file: GET /agents/{id}/workspace?path=SOUL.md
4. Ask the agent to edit: POST /agents/{id}/chat with:
     {
       "message": "Update SOUL.md to add a new capability: code review",
       "session": "autobot-manager-workspace-editor-chat"
     }
   You can append file context to your message to tell the agent which file you are
   looking at: "Update SOUL.md to add...\n<context>Currently viewing: SOUL.md</context>"
5. The agent edits the file in its workspace and responds with what it changed.
6. Verify the edit: GET /agents/{id}/workspace?path=SOUL.md — re-read the file
   to confirm the changes were applied.

You can send multiple chat messages on the same session for follow-up edits.
The agent retains the conversation context within the session.

Example — tell the agent to update its SOUL.md:
  curl -X POST %s/agents/my-agent-abc123/chat \
    -H "Authorization: Bearer <ADMIN_TOKEN>" \
    -H "Content-Type: application/json" \
    -d '{"message":"Add a section about code review to SOUL.md","session":"autobot-manager-workspace-editor-chat"}'

Example — verify the workspace after edits:
  curl "%s/agents/my-agent-abc123/workspace?path=SOUL.md" \
    -H "Authorization: Bearer <ADMIN_TOKEN>"
`

func (h *Handlers) LLMsTxt(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = fh
	}
	baseURL := scheme + "://" + host

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, llmsTxtTemplate,
		baseURL,  // Base URL header
		baseURL,  // POST /agent-registrations example
		baseURL,  // GET /agent-registrations example
		baseURL,  // PUT /agent-registrations/{name} example
		baseURL,  // PUT /agent-registrations/{name}/archive example
		baseURL,  // DELETE /agent-registrations/{name} example
		baseURL,  // POST /connections example
		baseURL,  // GET /connections example
		baseURL,  // DELETE /connections example
		baseURL,  // GET /agents example
		baseURL,  // GET /agents/{id} example
		baseURL,  // POST /agents/{id}/chat example
		baseURL,  // GET /agents/{id}/workspace example
		baseURL,  // GET /agents/{id}/workspace/locks example
		baseURL,  // PUT /agents/{id}/workspace/locks lock example
		baseURL,  // PUT /agents/{id}/workspace/locks unlock example
		baseURL,  // GET /agents/{id}/sessions example
		baseURL,  // POST /crons example
		baseURL,  // GET /crons example
		baseURL,  // GET /crons/{id} example
		baseURL,  // PUT /crons/{id} example
		baseURL,  // DELETE /crons/{id} example
		baseURL,  // POST /crons/{id}/trigger example
		baseURL,  // GET /crons/{id}/executions example
		baseURL,  // GET /metrics example
		baseURL,  // GET /metrics/series example
		baseURL,  // GET /logs example
		baseURL,  // GET /audit example
		baseURL,  // GET /health example
		baseURL,  // GET /events example
		baseURL,  // Workspace editor chat example
		baseURL,  // Workspace verify example
	)
}
