# Running the Multi-Agent Team

## Prerequisites

- Docker and Docker Compose
- An Anthropic API key

## 1. Download binaries

Download the latest release from [GitHub Releases](https://github.com/somit/clawmatrix/releases) and place the linux/amd64 binaries in `bin/`:

```
bin/clutch          # clutch sidecar (linux/amd64)
bin/control-plane   # control plane server (linux/amd64)
bin/picoclaw        # picoclaw agent runtime (linux/amd64)
```

## 2. Set your API key

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

## 3. Start the stack

```bash
docker compose up --build
```

Open the dashboard at **http://localhost:8080**.

All agents register automatically. Click any agent to chat with it.

## Agents

| Agent | Port | Runtime | Role |
|-------|------|---------|------|
| CEO | 9090 | picoclaw | Strategic decisions, delegates to team |
| Marketing Manager | 9091 | picoclaw | Marketing strategy and campaigns |
| Sales Manager | 9092 | picoclaw | Sales pipeline and outreach |
| CTO | 9093 | openclaw | Tech strategy, delegates to Tech Lead |
| Tech Lead | 9093 | openclaw | Technical analysis and research |

## Stop

```bash
docker compose down
```
