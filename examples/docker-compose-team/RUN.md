# Running the Multi-Agent Team

## Prerequisites

- Docker and Docker Compose
- A Cloudflare account ID and Workers AI API token

## 1. Download binaries

Download the latest release from [GitHub Releases](https://github.com/somit/clawmatrix/releases) and place the linux/amd64 binaries in `bin/`:

```
bin/clutch          # clutch sidecar (linux/amd64)
bin/control-plane   # control plane server (linux/amd64)
bin/picoclaw        # picoclaw agent runtime (linux/amd64)
```

The ADK agent image installs Google ADK from PyPI during `docker compose build`.

## 2. Set credentials

```bash
cp .env.example .env
# edit .env and set CLOUDFLARE_ACCOUNT_ID + CLOUDFLARE_API_TOKEN
# ADK agents use Vertex Gemini via local ADC:
gcloud auth application-default login
```

## 3. Start the stack

```bash
docker compose up --build
```

Open the dashboard at **http://localhost:8080**.

All agents register automatically. Click any agent to chat with it.

## Local control plane with air

If `control-plane` is already running locally with `air`, start only the agent containers and point them at the host:

```bash
docker compose -f docker-compose.local-control-plane.yml up --build
```

The local control-plane must load the same registrations:

```bash
BOOTSTRAP_CONFIG=../examples/docker-compose-team/config/bootstrap.json
```

Set `HOST_CONTROL_PLANE_URL` in `examples/docker-compose-team/.env` if your local control-plane is not reachable at `http://host.docker.internal:9999`.

## Agents

| Agent | Port | Runtime | Role |
|-------|------|---------|------|
| CEO | 9090 | picoclaw | Strategic decisions, delegates to team |
| Marketing Manager | 9091 | ADK | Marketing strategy and campaigns |
| Sales Manager | 9092 | ADK | Sales pipeline, outreach, and A2A calls to Marketing |
| CTO | 9093 | openclaw | Tech strategy, delegates to Tech Lead |
| Tech Lead | 9093 | openclaw | Technical analysis and research |

## Stop

```bash
docker compose down
```
