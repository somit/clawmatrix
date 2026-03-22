# Contributing to ClawMatrix

## Prerequisites

- Go 1.22+
- Docker + Docker Compose (for running examples)
- SQLite (default) or PostgreSQL

## Project layout

```
control-plane/   Go server — REST API, UI, scheduler, database
clutch/          Go sidecar — proxy, sniffer, agent gateway, CLI
examples/        Docker Compose examples
docs/            Additional documentation
```

## Running locally

```bash
# Control plane (SQLite, plain HTTP)
cd control-plane
JWT_SECRET=dev go run .
# First time: create an admin user
# JWT_SECRET=dev go run . createadmin --username admin --password admin

# Clutch (connects to local control plane)
cd clutch
go run . --control-plane http://localhost:8080 --token <registration-token>
```

## Building

```bash
# Control plane
cd control-plane && go build .

# Clutch
cd clutch && go build .

# Linux/amd64 (for Docker)
cd control-plane && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .
cd clutch        && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .
```

## Running the multi-agent example

See [`examples/docker-compose-multi-agent/RUN.md`](examples/docker-compose-multi-agent/RUN.md).

## Adding a new agent runtime

Clutch supports pluggable agent runtimes via the `AgentRunner` interface in `clutch/internal/runner.go`. Currently tested: **PicoClaw** and **OpenClaw**. **NanoClaw** is in progress.

To add support for a new runtime:

1. Create `clutch/internal/runner_<name>.go` implementing `AgentRunner` (`clutch/internal/runner.go`):
   - `CommandArgs(agent, msg, session)` — return the argv to spawn the subprocess
   - `UsesStdin() bool` — whether the message is passed via stdin
   - `Env() []string` — environment variables for the subprocess
   - `PrepareSession(agent, session)` — any pre-run setup (e.g. session file repair)
   - `ParseOutput(stdout, stderr)` — extract response, thinking, and usage from output
   - `AgentCmd(localID)` — command string stored on the agent record
   - `SessionsPath(localID)` — path to the sessions directory
   - `DiscoverAgents() []agentDiscovery` — return all agents from config (`nil` for single-agent runtimes)
2. Wire it up in the runner factory in `clutch/internal/runner.go` (switch on `RUNNER` env var)
3. Open a PR against `clutch/` with a short description of the runtime and how to test it

## Submitting changes

1. Fork the repo and create a branch
2. Make your changes — keep PRs focused and small
3. Ensure `go build ./...` passes in both `control-plane/` and `clutch/`
4. Open a pull request with a clear description of what and why
