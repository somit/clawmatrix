# Future: "Write code → upload → run" agents (ADK golden path)

> Status: **deferred / future work**. Captured for later — not started.
>
> **Positioning (decided):** clawmatrix is a **runtime-agnostic run/operate plane** — Shakudo-like control plane + gateway + governance, but bring-your-own-framework and self-hostable. The builder assistant is a feature, not the core. See [competitive-landscape.md](./competitive-landscape.md) for the market rationale.

## Context

Today clawmatrix is a **control plane + clutch sidecar** for AI-agent fleets. Its differentiation is being *runtime-agnostic*: the `AgentRunner` interface (`clutch/internal/runner.go`) already abstracts picoclaw, openclaw, `google-adk`, and generic stdin/stdout runtimes, and A2A delegation is wrapped at the control-plane layer. The current model is **bring-your-own-container, then register**.

We want a new capability: a place where people **write agent code, upload it, and run it** (Replit/Vercel-for-agents) layered on the *same* control plane.

Decisions made:
- **Hosting = hybrid.** clawmatrix can run the agent itself (on-ramp) *and* the user can self-host the same artifact and register it. Both reuse the existing registration path — no parallel runtime.
- **Framework = one golden path: Google ADK (Python).** openclaw/picoclaw/generic stay supported for BYO-container users via the existing runner interface. The upload/build experience targets ADK first only.
- **Artifact = source code (zip).** The platform builds the runnable image.

**Why this is cheap:** the runtime contract already exists. `clutch/internal/runner.go:48` has `case "google-adk"`, and ADK runs today via stdin/stdout (`examples/docker-compose-team/adk-sales/agent_cli.py`). On boot, clutch auto-registers via `REGISTRATION_TOKEN` (`clutch/internal/register.go`), which creates an `Agent` + upserts an `AgentProfile` — so anything that registers shows up in dashboard/A2A/cron/allowlists **for free**. `AgentProfile.Image`/`DeploymentConfig`/`Source` (`control-plane/internal/database/models.go:81-85`) already exist as placeholders for this. The only gap is the *front* of the pipeline: project ownership, upload, build, launch.

**Hybrid guarantee:** hosted and self-host use the *identical image* and *identical env contract* (proven in `examples/docker-compose-team/docker-compose.yml:88-114`), differing only in *who runs `docker run`*. Same artifact, same registration token, different launcher.

## Scope — Phase 1

ADK source **zip** → local `docker build` → hosted `docker run` → self-host download. Later phases (git import, Kaniko/k8s/Cloud Run, in-browser editor, multi-framework templates) slot in behind the same interfaces.

## Changes

### 1. Data model — `control-plane/internal/database/`
- Add `AgentProject` and `ProjectBuild` to `models.go`. `AgentProject{ Name, OwnerUserID, Framework('google-adk'), SourceType, SourceRef, EntryCmd('python /app/agent_cli.py'), ImageRef, Status, RegistrationName, RunHandle }`; `ProjectBuild{ ProjectID, Status, ImageRef, Logs, Error, StartedAt, FinishedAt }`.
- Register both in `DB.AutoMigrate(...)` at `db.go:40`.
- New file `database/projects.go` (mirror `repository.go`): `CreateAgentProject`, `GetAgentProject`, `ListAgentProjectsByOwner`, `UpdateAgentProjectStatus`, `CreateProjectBuild`, `AppendBuildLog`, `FinishProjectBuild`, `GetLatestBuild`.
- **Mapping onto existing model (no changes to Register/visibility):** at project-create, pre-create a `Registration` via existing `database.CreateRegistration` (`repository.go:19`) to mint the `REGISTRATION_TOKEN`; insert an `AgentProfileACL` (`models.go:34`) granting the owner the `owner` role so `ListAgents`' `VisibleProfiles` filter (`handlers.go:632`) shows it only to the owner. On build success set `AgentProfile.Image`/`Source="upload"`.

### 2. Upload + lifecycle API — `control-plane/internal/api/`
- New permission `PermManageProjects = "can_manage_projects"` in `permissions.go` (system-scoped; add to `allSystemPerms` + `admin` role).
- New file `api/projects.go` with `*Handlers` methods: `CreateProject`, `UploadSource` (multipart zip → stream to `DATA_DIR/projects/<id>/source.zip`, validate entry file, cap size), `BuildProject`, `GetBuild`, `StreamBuildLogs` (reuse SSE flusher from `ChatProxy` `handlers.go:919`), `ListProjects`, `GetProject`, `DeleteProject`, `RunProject`, `StopProject`, `DownloadArtifact`. `{id}` handlers enforce `OwnerUserID == userFromCtx(r).ID` (admins bypass), mirroring `checkProfilePerm` (`middleware.go:78`).
- Wire routes in `routes.go` after the Registrations block (~line 69), behind `withPerm`/`withAuth` + ownership.
- Add config to `config.go` via the `envOr` pattern: `DataDir` (`/data`), `Builder` (`docker`), `AgentImageRegistry`, `AdkBaseImage` (`python:3.12-slim`), `HostedNetwork`.

### 3. Build pipeline — new `control-plane/internal/builder/`
- **User-code contract** (document + validate on upload, generalize `agent_cli.py`): entry script reads message from **stdin**, accepts `--session`, prints response to **stdout**, exits 0; default `/app/agent_cli.py`; optional `clawmatrix.yaml` (`entry:`, `requirements:`) override. The consuming half (`googleAdkRunner`/generic) already exists — no runtime code.
- Parametrized Dockerfile template embedded via `//go:embed` at `builder/templates/adk.Dockerfile.tmpl`, derived from `examples/docker-compose-team/Dockerfile.adk`: base image + `google-adk[a2a]` + copy `clutch` + `entrypoint.sh` + `COPY app/ /app/` + conditional `pip install -r requirements`. Runner type/agent-cmd are **not** baked — passed as runtime env so the image is identical hosted vs self-host.
- Swappable `Builder` interface: `Build(ctx, BuildRequest{ProjectID, SourceDir, ImageTag, Dockerfile}, logs io.Writer) (imageRef, error)`. `builder/docker.go` (Phase 1) assembles a context dir and shells out to `docker build` via `exec.CommandContext`, streaming combined output into `logs`. `NewBuilder(cfg)` factory mirrors clutch's `newRunner()` switch (`runner.go:42`); kaniko/cloudrun are later impls.
- Orchestration: goroutine (next to `worker/stale.go`) extracts source → renders template → builds → `AppendBuildLog` + broadcast `build:log` events via the existing SSE hub. On success: set build/project/profile image + status; on failure: store `Error`.

### 4. Hosted run path — new `control-plane/internal/launcher/`
- Swappable `Launcher` interface: `Launch(ctx, LaunchSpec) (handle, error)`, `Stop`, `Status`. `launcher/docker.go` (Phase 1) runs the image with the **exact env block from `docker-compose.yml:98-110`**: `CONTROL_PLANE_URL`, `REGISTRATION_TOKEN` (project's), `AGENT_ID`, `AGENT_GROUP=<name>`, `RUNNER=google-adk`, `AGENT_CMD=<EntryCmd>`, `WORKSPACE_PATH`, `SESSIONS_PATH`, `HOST_URL`. Clutch boots PID 1, auto-registers, agent appears in dashboard — **zero control-plane special-casing**. Store handle in `AgentProject.RunHandle`; `StopProject` calls `Launcher.Stop`. k8s/Cloud Run slot in behind the same interface later.
- **Untrusted-code posture (Phase 1, pragmatic):** trusted-user on-ramp on a dedicated docker host. Run with `--read-only` root + writable `/workspace`, `--cap-drop=ALL`, `--pids-limit`, mem/CPU limits, no host mounts, egress allowlist via clutch. Set `DISABLE_SNIFFER=true` (supported `clutch/main.go:202`) for hosted builds to avoid the `NET_ADMIN`/iptables requirement. Strong multi-tenant isolation (rootless/Kaniko builds, gVisor/Firecracker, per-tenant namespaces) is Phase 3.

### 5. Self-host path
- `DownloadArtifact` returns a zip with a `docker compose` snippet (the `sales-manager` stanza from `docker-compose.yml:88` with project values substituted) + the **same** registration token, pointing at the same image (registry pull, or `docker save` tarball for fully-local Phase 1). The self-hosted container registers into the same `Registration` and appears in the same dashboard.

### 6. UI
- Add a "Projects" tab in `control-plane/internal/ui/` (templ): create project, upload zip, build (live logs via SSE), run/stop, download self-host bundle, link to the resulting agent.

### 7. LLM settings (chosen during creation)

The user picks the agent's LLM config in the create flow (in the Projects UI / builder assistant). This config is stored on the project and injected as runtime env to the launched container — it is **not** baked into the image (keeps hosted vs self-host image identical, §4/§5).

- **Stored on `AgentProject`** (extend the model in §1): `LLMProvider` (e.g. `anthropic`, `vertex`, `cloudflare`, or the local `llm-proxy`), `LLMModel`, and a reference to the credential (not the raw key — store secrets separately).
- **Secrets:** add a minimal per-project secrets store (encrypted column or a `ProjectSecret` table) rather than putting API keys in `AgentProject`. The launcher (§4) and self-host bundle (§5) read these and surface them as env vars (e.g. `ANTHROPIC_API_KEY`, `CLOUDFLARE_ACCOUNT_ID`/`CLOUDFLARE_API_TOKEN` as `agent_cli.py` already expects, or `LLM_PROXY_URL` pointing at the existing llm-proxy on `:8081`).
- **Default golden path:** route through the existing **llm-proxy** (`/anthropic/*`, `/vertex/*`) so the platform owns provider auth and the user just picks a model — avoids handling raw provider keys for hosted agents. Direct-provider keys remain an option for self-host / BYO-key.
- **llm-proxy = the LLM/policy plane (decided).** Keep it a **separate Go module / repo** (it's ~18k LOC with its own multi-tenant DB, dashboard, billing — don't merge into the control-plane module), but treat it as a first-class clawmatrix service: ship it as a compose service in the examples, have the launcher (§4) inject `LLM_PROXY_URL`, and let it own provider auth + audit. This mirrors Shakudo's "AI Gateway" pattern (see [competitive-landscape.md](./competitive-landscape.md)). Two repos, one product.
- **Injection:** add the chosen env to `LaunchSpec.Env` (§4) and to the downloaded compose snippet (§5), so hosted and self-host get identical LLM wiring.
- The builder assistant (§7 below) presents the model/provider choice as part of the interview and writes it onto the project before build/run.

### 8. AI builder assistant (agent that helps create agents)

An AI agent assists the user through the whole create flow — idea → working ADK agent — instead of the user hand-writing `agent_cli.py`. Key framing: **the builder is itself just another clawmatrix agent (dogfood the ADK golden path), with tools wired to the Phase-1 project API.** No special runtime; it's an agent whose tools are the `/projects/*` endpoints.

- **What it does (conversational, agentic loop):**
  1. Interview the user for intent (role, tools the agent needs, who it delegates to via A2A).
  2. Scaffold from the ADK template: generate `agent_cli.py` (honoring the stdin/stdout `--session` contract — see §3 user-code contract), `requirements.txt`, and `clawmatrix.yaml`.
  3. Create the project (`POST /projects`), upload the generated source (`POST /projects/{id}/source`), trigger `POST /projects/{id}/build`.
  4. **Build-fix loop:** stream `GET /projects/{id}/builds/{buildId}/logs`, and on failure edit the source and re-upload/rebuild until green.
  5. Smoke-test: `POST /projects/{id}/run` then `POST /agents/{id}/chat` with a sample message; show the user the response and iterate.
- **Tools surface:** a small toolset mapping 1:1 to the project API (`create_project`, `write_source`, `build`, `read_build_logs`, `run`, `test_message`). Reuse the A2A/agent-card discovery so the builder can wire the new agent's delegation connections.
- **Auth:** acts on behalf of the signed-in user with a scoped token; projects it creates are owned by that user (same `AgentProfileACL` owner-role path as §1). The builder must not be able to manage other users' projects — enforce the same `OwnerUserID` check (§2).
- **UI:** a chat panel inside the "Projects" tab (§6) that drives this loop; build logs and the test response render inline.
- **Phasing:** depends on the Phase-1 API existing, so it lands as **Phase 2** (after upload/build/run works headless). The build-fix loop and connection-wiring are the highest-value parts; keep the first cut to scaffold → upload → build → one test message.
- **Guardrails:** generated code still passes the §3 upload validation; the builder cannot bypass the untrusted-code run posture (§4) — its agents launch through the same sandboxed launcher.

## Verification (end-to-end)
1. `POST /projects {name:"demo-sales", framework:"google-adk"}` → returns id + registration token.
2. `POST /projects/{id}/source` with a zip of `examples/docker-compose-team/adk-sales/` (entry at `/app/agent_cli.py`) → status `uploaded`.
3. `POST /projects/{id}/build` → poll `GET /projects/{id}/builds/{buildId}` to `success`; confirm `docker images` shows the tag.
4. `POST /projects/{id}/run` → `docker ps` shows the container; clutch logs show "registered 1 agents" (`register.go:179`).
5. `GET /agents` returns the agent, visible only to the owner via ACL.
6. `POST /agents/{id}/chat {message:"qualify this lead"}` → `ChatProxy` (`handlers.go:853`) reaches the container, ADK responds; A2A delegation works (token/CP URL present, `agent_cli.py:116`).
7. `GET /projects/{id}/artifact` → run the bundle on a laptop with the same token → a second self-hosted instance registers into the same `Registration` and appears in the dashboard. **Same artifact, different launcher — confirmed.**

## Out of scope (later phases)
Git URL import (P2) · registry push for remote self-host (P2) · Kaniko/rootless builds + k8s/Cloud Run launchers + strong tenant isolation (P3) · in-browser editor + multi-framework build templates selected by `AgentProject.Framework` (P4).
