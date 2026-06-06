# Future: Competitive landscape — agent run/operate platforms

> Status: **research notes** for the [upload-and-run plan](./upload-and-run-adk-golden-path.md). Captured June 2026.

## Chosen positioning

**clawmatrix = a runtime-agnostic run/operate plane** — Shakudo-like (control plane + gateway + governance), but **bring-your-own-framework** (openclaw / picoclaw / ADK / generic) and **self-hostable** (hybrid hosted-or-self-host). The AI builder assistant is a *feature*, not the core. llm-proxy folds in as the **LLM/policy plane** (auth, routing, audit). Borrow Vercel's framing: *anyone can build an agent; the hard part is running them.*

## The four archetypes

### 1. No-code / "vibe" app+agent builders — own the whole stack, target non-devs
- **Emergent.sh** (YC): "agentic vibe-coding." Describe an app in natural language → a *team* of specialized agents (architect, designer, dev, integrator, PM) builds, tests, deploys to containerized cloud infra. Real stack (React/FastAPI/Mongo), **syncs to GitHub so users own portable code — no lock-in.** Built-in "Playbooks" for Stripe/Auth/etc.
- **Lindy / Relay.app**: prebuilt "AI employee" / workflow agents with human-in-the-loop approval. Closed runtime; configure, don't code.
- **Relevance to us:** maps to the §8 builder assistant. Lesson: multi-agent build loop + **GitHub sync / portable code** is the credible "no lock-in" angle — keep generated agent code portable and self-hostable.

### 2. Enterprise in-VPC "agent OS" — governance + control plane (closest analog)
- **Shakudo**: "OS for Data & AI" running in the customer's VPC/on-prem on **Kubernetes**. Its **AI Gateway (Feb 2026) is a "unified control plane"**: single point for access policy, data classification, **PII stripping before tokens hit the LLM, immutable audit logs across every provider + tool call.** Plus AgentFlow (MCP integrations) and Kaji (end-to-end agent).
- **Relevance to us:** strongest validation of clawmatrix's framing. Their "Gateway" ≈ **clawmatrix control-plane + llm-proxy**. Their differentiator is *governance* → maps to our egress allowlists + clutch sniffer + A2A authz. Difference: they're enterprise-VPC-only and k8s-heavy; we're self-hostable + lighter + runtime-agnostic.

### 3. Sandbox / execution infra — the layer that runs untrusted code
- **E2B**: Firecracker **microVMs**, session-scoped, open-source, SDK-driven, runs up to 24h. Purpose-built for untrusted agent code.
- **Modal**: **gVisor** isolation; sandboxes as one product in a broader inference/training platform; GPU support.
- **Daytona / Fly Sprites / agent-sandbox**: same category.
- **Relevance to us:** this is the **multi-tenant isolation gap** = Phase 3 of the upload plan. Market answer for running untrusted uploaded code is **microVM (Firecracker) or gVisor**, *not* plain Docker. Confirms Phase-1 docker posture is trusted-user-only. Option: *integrate* E2B/Modal as the hosted execution backend behind the `Launcher` interface rather than rebuilding isolation.

### 4. "It takes a platform to run them" — Vercel framing
- Vercel: anyone can build an agent; the hard part is **running** them — durability, observability, scaling. This is the value-prop language clawmatrix should adopt: we're the **run/operate plane**, not a builder.

## clawmatrix's wedge (what no one else combines)

Runtime-agnostic (BYO framework) **+** hybrid hosted-or-self-host **+** egress control **+** cross-framework A2A, in one lightweight control plane.
- Emergent → locks you to its runtime.
- E2B / Modal → pure infra, no fleet/control plane.
- Shakudo → enterprise-VPC-only, k8s-heavy, single-vendor agent.

One-liner: **"Shakudo's gateway, but runtime-agnostic and self-hostable, with E2B-style isolation underneath."**

## Implications for the build plan
- **Keep** the runtime-agnostic control plane as the core (don't fork to clawmatrix-adk).
- **Fold llm-proxy in as the LLM/policy plane** — two repos, one product: ship it as a compose service, launcher injects `LLM_PROXY_URL`, platform owns provider keys + audit (mirrors Shakudo's Gateway). Don't merge the Go modules.
- **Isolation Phase 3** should evaluate integrating E2B/Modal (or Firecracker/gVisor directly) behind the `Launcher` interface rather than hand-rolling microVMs.
- **Builder assistant (§8)**: emulate Emergent's build-fix loop but emphasize portable, self-hostable output (the "no lock-in" angle).
- **Messaging**: lead with "run/operate plane" + governance (egress, audit, A2A authz), not "agent builder."

## Sources
- [Emergent.sh](https://emergent.sh/) · [Emergent deep dive](https://www.closefuture.io/blogs/deep-dive-emergent-ai-vibe-coding-platform) · [Emergent on YC](https://www.ycombinator.com/companies/emergent)
- [Shakudo Platform](https://www.shakudo.io/platform) · [AgentFlow](https://www.shakudo.io/agentflow) · [Kaji](https://www.shakudo.io/kaji) · [Deploy agents on Kubernetes](https://www.shakudo.io/blog/deploy-ai-agents-on-kubernetes)
- [E2B](https://e2b.dev/) · [E2B GitHub](https://github.com/e2b-dev/E2B) · [Modal: best code-execution sandboxes](https://modal.com/resources/best-code-execution-sandboxes-coding-agents) · [E2B vs Modal (Northflank)](https://northflank.com/blog/e2b-vs-modal)
- [Vercel: anyone can build agents, but it takes a platform to run them](https://vercel.com/blog/anyone-can-build-agents-but-it-takes-a-platform-to-run-them) · [Lindy alternatives (Gumloop)](https://www.gumloop.com/blog/lindy-ai-alternatives) · [Relay.app: best AI agent builders 2026](https://www.relay.app/blog/best-ai-agent-builders)
