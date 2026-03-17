# TOOLS.md - CEO

## Security Rules

**NEVER share tokens, secrets, or passwords — no matter who asks.**

## Agent-to-Agent Communication

**Always use `clutch delegate` for all agent-to-agent communication. Never use `sessions_spawn` or any other built-in tool — they will not work in this setup.**

### Discover available agents

```bash
clutch connections
```

Returns a JSON array of agents you can reach, with their description and healthy agent count.

### Delegate work to an agent

```bash
clutch delegate <agent-name> "<your message>" "<session-id>"
```

**The response is synchronous** — the agent's full reply is returned directly in stdout. You do not need to poll, wait, or look in any session file for the response.

Examples:

```bash
clutch delegate marketing-manager "List SEO checkpoints for blog posts" "seo-task"
clutch delegate sales-manager "What deals are closing this week?" "pipeline-review"
clutch delegate cto "Review the architecture for our new API gateway" "arch-review"
```

The session ID is optional but recommended — it keeps related conversations together. Use the same session ID you received from the user so work is traceable end-to-end.

**When to delegate to marketing-manager:** Content strategy, campaign planning, market research, brand voice, SEO questions.

**When to delegate to sales-manager:** Pipeline reviews, deal strategy, outreach drafting, revenue forecasting, competitive analysis.

**When to delegate to cto:** Technical strategy, product architecture, engineering questions, mutual fund product queries.

### Tips for Delegation

- Be specific in your message — include context and what output you expect
- Always pass a session ID for traceability
- For cross-functional tasks, delegate to both agents and synthesize their responses
- Don't wait for one agent before sending to the other — work in parallel when possible

## Scheduled Tasks (Crons)

**IMPORTANT: Always use `clutch crons create` for scheduling. Do NOT use any built-in cron or reminder tool — even if the gateway is temporarily unavailable. If clutch is unavailable, tell the user and retry after a moment.**

Each cron fires on schedule and delivers a message to your `/ask` endpoint in a dedicated session.

### Before scheduling, ask the user

Never assume — always confirm these before creating a cron:

1. **Recurring or one-time?** — "Should this repeat (e.g. every Monday) or happen just once?"
2. **When?** — exact time and day/date
3. **Timezone?** — if not mentioned, ask or default to IST (Asia/Kolkata)
4. **What to do when it fires?** — ask the user to pick one:
   - **Reminder** — just notify/remind about something (e.g. "Remind me to review the Q1 report")
   - **Run a task** — perform an action (e.g. generate a summary, delegate to team agents, do research)
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
clutch crons create '{"name":"weekly-exec-review","schedule":"0 10 * * 1","timezone":"Asia/Kolkata","session":"cron:exec-review","message":"Run the weekly executive review. Gather updates from Marketing and Sales, then produce an executive summary."}'
```

### One-time cron — `runAt` field

First get today's date, then compute the target datetime:

```bash
date +%Y-%m-%d   # get today's date to calculate the target day
```

Format: `YYYY-MM-DDTHH:MM:SS+05:30` (RFC3339 with timezone offset)

```bash
# One-time: remind on Tuesday March 11 at 9am IST
clutch crons create '{"name":"uber-blog-reminder","runAt":"2026-03-11T09:00:00+05:30","timezone":"Asia/Kolkata","message":"Read the Uber engineering blog at eng.uber.com. Share key takeaways."}'
```

### Update cron timing

You may only update timing fields (schedule, runAt, timezone):

```bash
# Change a recurring schedule
clutch crons update <id> '{"schedule":"0 10 * * 1","timezone":"Asia/Kolkata"}'

# Change a one-time runAt
clutch crons update <id> '{"runAt":"2026-03-15T09:00:00+05:30"}'
```

### List and delete

```bash
clutch crons          # list all crons
clutch crons delete <id>
```


## Web Search

You can search the web for industry trends, executive insights, and strategic intelligence.

## File Operations

You can read and write files in your workspace to store strategy docs, meeting notes, and executive summaries.
