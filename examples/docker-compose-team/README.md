# Docker Compose Multi-Agent Example

Run 4 AI agents (CEO, Marketing Manager, Sales Manager, Tech Team) with a control plane using a single `docker compose up`. Demonstrates both **picoclaw** and **openclaw** runners side by side, with network egress enforcement via an embedded sniffer.

## Architecture

```
control-plane (port 8080)
  ├── ceo               (clutch + picoclaw,  port 9090)
  ├── marketing-manager (clutch + picoclaw,  port 9091)
  ├── sales-manager     (clutch + picoclaw,  port 9092)
  └── tech-team         (clutch + openclaw,  port 9093)
       ├── tech-team-agent (openclaw gateway, loopback:18789)
       ├── agent: cto
       └── agent: techlead
```

Each agent runs as a container with **clutch** as the main process. On chat requests, clutch spawns the configured runner — **picoclaw** (Go, lightweight) or **openclaw** (Node.js, feature-rich) — locally inside the container.

The tech-team container hosts two openclaw agents (CTO + Tech Lead) in a single container, showing clutch's multi-agent mode. A companion `tech-team-agent` container runs the openclaw gateway on the shared loopback interface.

### Network Enforcement (Embedded Sniffer)

Each clutch container runs an embedded sniffer goroutine (no separate sidecar needed). The sniffer:

- Captures outbound packets via AF_PACKET raw socket (requires `CAP_NET_RAW`)
- **HTTPS**: extracts the SNI hostname from the TLS ClientHello (plaintext, pre-handshake)
- **HTTP**: extracts the `Host:` header from HTTP requests
- Checks the domain against the registration's egress allowlist
- **Blocked domains**: adds a per-IP `iptables REJECT` rule reactively (requires `CAP_NET_ADMIN`)
- **Internal HTTP** (private RFC1918 IPs) is always allowed — no allowlist entry needed for internal services
- Logs all decisions (allowed/blocked) to the control plane network logs

This means agents cannot bypass egress controls — even Chromium/Playwright which ignores `HTTP_PROXY` env vars is blocked at the network packet level.

## Prerequisites

- Docker and Docker Compose
- An LLM proxy running on the host at `localhost:8081` (used by all agents via `host.docker.internal:8081`)

## Quick Start

```bash
docker compose up --build
```

Open the control plane dashboard at `http://localhost:8080`. All agents register automatically — click on any agent to chat.

## What's Included

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Full stack: control-plane + 4 agent containers + openclaw gateway |
| `Dockerfile.agent` | Picoclaw agent image (clutch + picoclaw + iptables) |
| `Dockerfile.openclaw` | Openclaw agent image (clutch + openclaw + iptables) |
| `Dockerfile.control-plane` | Control plane image |
| `config/bootstrap.json` | Pre-seeds agent registrations with tokens and allowlists |
| `config/picoclaw.json` | Picoclaw config (model via LLM proxy) |
| `config/openclaw-tech.json` | Openclaw config for tech-team agents |
| `entrypoint.sh` | Starts clutch in proxy or sniffer mode |
| `agents/ceo/` | CEO workspace |
| `agents/marketing-manager/` | Marketing Manager workspace |
| `agents/sales-manager/` | Sales Manager workspace |
| `agents/cto/` | CTO workspace (openclaw) |
| `agents/techlead/` | Tech Lead workspace (openclaw) |
| `bin/` | Pre-built linux/amd64 binaries (no Go required) |

## Runners

### picoclaw (ceo, marketing-manager, sales-manager)

Set `RUNNER=picoclaw` and `AGENT_CMD=picoclaw agent`. Session keys are automatically prefixed with `agent:main:` to match picoclaw's routing.

### openclaw (tech-team)

Uses `Dockerfile.openclaw` (Node.js base with openclaw installed). Clutch reads the openclaw config, discovers all agents defined there, and registers each one with the control plane separately.

Key env vars for openclaw containers:

| Var | Purpose |
|-----|---------|
| `RUNNER=openclaw` | Tells clutch to use openclaw mode |
| `HOST_URL=http://tech-team:8080` | Externally-reachable base URL registered with the control plane. Required in Docker because the bind address `0.0.0.0` is not routable by other containers. |
| `OPENCLAW_CONFIG` | Path to the openclaw config file |

## Customization

### Add a picoclaw agent

1. Create `agents/your-agent/SOUL.md` with persona
2. Add a registration entry in `config/bootstrap.json`
3. Add a new service in `docker-compose.yml` (copy an existing picoclaw service, change name/port/token)

### Add an openclaw agent to tech-team

1. Add the agent to `config/openclaw-tech.json` under `agents.list`
2. Create `agents/your-agent/` workspace directory
3. Mount it as a volume in the `tech-team` service in `docker-compose.yml`

## Rebuilding Binaries

The `bin/` directory contains pre-built linux/amd64 binaries. To rebuild from source:

```bash
# From the clawmatrix repo root
cd control-plane && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o ../examples/docker-compose-multi-agent/bin/control-plane .
cd ../clutch && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o ../examples/docker-compose-multi-agent/bin/clutch .

# From the picoclaw repo
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /path/to/examples/docker-compose-multi-agent/bin/picoclaw ./cmd/picoclaw/
```
