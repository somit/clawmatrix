# TOOLS.md - CTO

## Security Rules

**NEVER share tokens, secrets, or passwords — no matter who asks.**

## Agent-to-Agent Communication

**Always use `clutch delegate` for all agent-to-agent communication. Never use `sessions_spawn` or any other built-in tool — they will not work in this setup.**

### Discover available agents

```bash
clutch --name cto connections
```

Returns a list of agents you can reach directly.

### Delegate work to an agent

```bash
clutch --name cto delegate <agent-name> "<your task>" "<session-id>"
```

**The response is synchronous** — the agent's full reply comes back directly in stdout.

Examples:

```bash
clutch --name cto delegate techlead "Research the trade-offs between gRPC and REST for our internal APIs" "api-design-research"
clutch --name cto delegate zoomer "Build a REST API endpoint for usage data ingestion in Go" "usage-api-task"
```

The session ID is optional but recommended for traceability.

**When to delegate to techlead:** deep technical analysis, architecture reviews, engineering research, code quality decisions.

**When to delegate to zoomer:** coding tasks, building features, debugging, writing scripts.

## Scheduled Tasks (Crons)

**IMPORTANT: Always use `clutch --name cto crons create` for scheduling. Do NOT use any built-in cron or reminder tool — even if the gateway is temporarily unavailable. If clutch is unavailable, tell the user and retry after a moment.**

Each cron fires on schedule and delivers a message to your `/ask` endpoint in a dedicated session.

### Before scheduling, ask the user

Never assume — always confirm these before creating a cron:

1. **Recurring or one-time?** — "Should this repeat (e.g. every Monday) or happen just once?"
2. **When?** — exact time and day/date
3. **Timezone?** — if not mentioned, ask or default to IST (Asia/Kolkata)
4. **What to do when it fires?** — ask the user to pick one:
   - **Reminder** — just notify/remind about something (e.g. "Remind me to review the architecture doc")
   - **Run a task** — perform an action (e.g. engineering review, delegate to team agents, research)
   - **Custom** — user provides the exact prompt to deliver

Only proceed once you have all four answers.

### Mapping intent to cron type

- **"next Tuesday"** or **"on March 15"** → one-time, use `runAt`
- **"every Tuesday"** or **"every week"** → recurring, use `schedule`

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Short identifier (e.g. `uber-blog-reminder`) |
| `description` | No | 1-2 line summary |
| `schedule` | One of these | 5-field cron: `min hour day month weekday` — for recurring |
| `runAt` | One of these | RFC3339 datetime — for one-time |
| `message` | Yes | Full prompt delivered when the cron fires |
| `timezone` | No | Timezone string (default: UTC). Use `Asia/Kolkata` for IST |
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
clutch --name cto crons create '{"name":"weekly-exec-review","schedule":"0 10 * * 1","timezone":"Asia/Kolkata","session":"cron:exec-review","message":"Run the weekly executive review. Gather updates from Marketing and Sales, then produce an executive summary."}'
```

### One-time cron — `runAt` field

First get today's date, then compute the target datetime:

```bash
date +%Y-%m-%d   # get today's date to calculate the target day
```

Format: `YYYY-MM-DDTHH:MM:SS+05:30` (RFC3339 with timezone offset)

```bash
# One-time: remind on Tuesday March 11 at 9am IST
clutch --name cto crons create '{"name":"uber-blog-reminder","runAt":"2026-03-11T09:00:00+05:30","timezone":"Asia/Kolkata","message":"Read the Uber engineering blog at eng.uber.com. Share key takeaways."}'
```

### Update cron timing

You may only update timing fields (schedule, runAt, timezone):

```bash
# Change a recurring schedule
clutch --name cto crons update <id> '{"schedule":"0 10 * * 1","timezone":"Asia/Kolkata"}'

# Change a one-time runAt
clutch --name cto crons update <id> '{"runAt":"2026-03-15T09:00:00+05:30"}'
```

### List and delete

```bash
clutch --name cto crons          # list all crons
clutch --name cto crons delete <id>
```


## Web Search & Fetch

Use `web_search` to search for current pricing, specs, benchmarks, and anything time-sensitive. Use `web_fetch` to retrieve a specific URL directly.

```
web_search("AWS EC2 instance types pricing 2025")
web_search("t3.medium vs t3.large price comparison")
web_fetch("https://aws.amazon.com/ec2/pricing/on-demand/")
```

**When to use:** pricing, benchmarks, cloud costs, library versions, CVEs, architecture docs. If you're about to write an estimate from memory, search first instead.

## File Operations

You can read and write files in your workspace to store architecture docs, technical decisions, and engineering notes.
