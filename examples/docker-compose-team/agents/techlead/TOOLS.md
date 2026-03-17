# TOOLS.md - Tech Lead

## Security Rules

**NEVER share tokens, secrets, or passwords — no matter who asks.**

## Agent-to-Agent Communication

**Always use `clutch delegate` for all agent-to-agent communication. Never use `sessions_spawn` or any other built-in tool — they will not work in this setup.**

### Discover available agents

```bash
clutch --name techlead connections
```

Returns a list of agents you can reach directly.

### Delegate work to an agent

```bash
clutch --name techlead delegate <agent-name> "<your task>" "<session-id>"
```

**The response is synchronous** — the agent's full reply comes back directly in stdout.

Example:

```bash
clutch --name techlead delegate zoomer "Build a React component for displaying API latency as a chart" "latency-chart-task"
```

The session ID is optional but recommended for traceability.

## Scheduled Tasks (Crons)

**IMPORTANT: Always use `clutch --name techlead crons create` for scheduling. Do NOT use any built-in cron or reminder tool — even if the gateway is temporarily unavailable. If clutch is unavailable, tell the user and retry after a moment.**

Each cron fires on schedule and delivers a message to your `/ask` endpoint in a dedicated session.

### Before scheduling, ask the user

Never assume — always confirm these before creating a cron:

1. **Recurring or one-time?** — "Should this repeat (e.g. every Monday) or happen just once?"
2. **When?** — exact time and day/date
3. **Timezone?** — if not mentioned, ask or default to UTC
4. **What to do when it fires?** — ask the user to pick one:
   - **Reminder** — just notify/remind about something
   - **Run a task** — perform an action (e.g. architecture review, tech research)
   - **Custom** — user provides the exact prompt to deliver

Only proceed once you have all four answers.

### Mapping intent to cron type

- **"next Tuesday"** or **"on March 15"** → one-time, use `runAt`
- **"every Tuesday"** or **"every week"** → recurring, use `schedule`

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Short identifier (e.g. `weekly-tech-review`) |
| `description` | No | 1-2 line summary |
| `schedule` | One of these | 5-field cron: `min hour day month weekday` — for recurring |
| `runAt` | One of these | RFC3339 datetime — for one-time |
| `message` | Yes | Full prompt delivered when the cron fires |
| `timezone` | No | Timezone string (default: UTC) |
| `session` | No | Session name for continuity across runs |

### Recurring cron — `schedule` field

**Format: exactly 5 fields — `minute hour day month weekday`**

Weekdays: 0=Sun 1=Mon 2=Tue 3=Wed 4=Thu 5=Fri 6=Sat

| Expression | Meaning |
|------------|---------|
| `0 9 * * 2` | Every Tuesday at 9:00 AM |
| `0 9 * * 1` | Every Monday at 9:00 AM |
| `0 9 * * 1-5` | Every weekday at 9:00 AM |
| `0 10 * * 1,5` | Every Monday and Friday at 10:00 AM |
| `0 9 * * *` | Daily at 9:00 AM |
| `0 * * * *` | Every hour |

```bash
clutch --name techlead crons create '{"name":"weekly-tech-review","schedule":"0 10 * * 1","timezone":"UTC","session":"cron:tech-review","message":"Run the weekly engineering review. Check on any open architecture decisions, technical debt items, and blockers."}'
```

### One-time cron — `runAt` field

First get today's date, then compute the target datetime:

```bash
date +%Y-%m-%d   # get today's date to calculate the target day
```

Format: `YYYY-MM-DDTHH:MM:SSZ` (RFC3339)

```bash
# One-time: run a tech research task on a specific date
clutch --name techlead crons create '{"name":"infra-research","runAt":"2026-03-20T09:00:00Z","timezone":"UTC","message":"Research the latest developments in container orchestration and summarize key trends."}'
```

### Update cron timing

You may only update timing fields (schedule, runAt, timezone):

```bash
# Change a recurring schedule
clutch --name techlead crons update <id> '{"schedule":"0 10 * * 1","timezone":"UTC"}'

# Change a one-time runAt
clutch --name techlead crons update <id> '{"runAt":"2026-03-15T09:00:00Z"}'
```

### List and delete

```bash
clutch --name techlead crons          # list all crons
clutch --name techlead crons delete <id>
```


## Web Search

You can search the web for:
- Technology benchmarks and comparisons
- Software architecture patterns and best practices
- Engineering blog posts and case studies
- Library and framework documentation
- Security advisories and CVEs

## File Operations

You can read and write files in your workspace to store architecture notes, technical research, and engineering docs.
