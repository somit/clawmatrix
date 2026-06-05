# User Management, Roles & Access

## Overview

ClawMatrix uses Role-Based Access Control (RBAC). Every access decision is the
same triple:

> **principal** (who) → **role** (what they can do) → **resource** (on what)

- **Permissions** are fixed string constants defined in code.
- **Roles** are bundles of permissions, stored in the DB. Default roles are
  seeded on startup; admins can create custom ones.
- **Principals** are **users** or **teams** (human groups). A user's access is the
  union of their own grants and the grants of every team they belong to.
- **Resources** are **agent profiles** or **individual agents**. A grant on a
  profile is inherited by all of its agents; a grant on an agent applies to that
  one agent only. The two compose **additively**.

---

## Permissions

Fixed strings defined in code (`internal/database/permissions.go`), grouped by scope.

### System-scoped (global)
| Permission | Description |
|---|---|
| `can_manage_users` | Create, edit, delete users |
| `can_manage_roles` | Create, edit, delete roles |
| `can_manage_registrations` | Create, edit, delete agent registrations |
| `can_manage_profiles` | Create/edit/delete agent profiles **and manage all access** (ACLs + teams) |
| `can_manage_connections` | Manage agent-to-agent connections |
| `can_manage_crons` | Manage cron jobs (admin scope) |
| `can_view_logs` | View request logs |
| `can_view_audit` | View audit trail |
| `can_view_metrics` | View metrics and stats |

> `can_manage_profiles` is the gate for the whole access subsystem: granting
> users/teams access to profiles or agents, and creating/managing teams.

### Profile-scoped (per profile **or** per agent)
These are checked against access bindings on a resource (a profile or an agent).
| Permission | Description |
|---|---|
| `can_view_agents` | See agents and their details |
| `can_chat_with_agents` | Use agent chat |
| `can_configure_agents` | Workspace writes, status/config changes |
| `can_view_crons` | View cron jobs and executions |
| `can_trigger_crons` | Manually trigger a cron job |
| `can_add_crons` | Create new cron jobs |
| `can_edit_crons` | Edit existing cron jobs |
| `can_delete_crons` | Delete cron jobs |

---

## Default Roles (seeded)

### System-scoped
| Role | Permissions |
|---|---|
| `admin` | all system permissions |

### Profile-scoped (usable on a profile or an agent)
| Role | Permissions |
|---|---|
| `owner` | all profile permissions |
| `operator` | view, chat, configure + all cron perms |
| `chatter` | `can_view_agents`, `can_chat_with_agents`, `can_view_crons`, `can_trigger_crons` |
| `viewer` | `can_view_agents`, `can_view_crons` |

Default roles have `system_default = true` and cannot be deleted or edited
(custom roles can). The same profile-scoped role can be granted on a profile or
on a single agent — the permission set is identical.

---

## Data Model

```
users
  id               uint (PK)
  username         string (unique)
  password_hash    string
  email            string (unique, nullable)
  system_role_id   uint FK → roles.id (nullable — no system role = member only)

roles
  id               uint (PK)
  name             string (unique)
  description      string
  scope            enum: system | profile
  system_default   bool   -- seeded roles, cannot be deleted/edited

role_permissions
  role_id          uint FK → roles.id
  permission       string
  PK: (role_id, permission)

access_bindings                       -- the single source of truth for resource ACLs
  id               uint (PK)
  principal_type   enum: user | group
  principal_id     uint   -- users.id or human_groups.id
  resource_type    enum: profile | agent
  resource_id      string -- profile name or agent id
  role_id          uint FK → roles.id  (profile-scoped role)
  UNIQUE (principal_type, principal_id, resource_type, resource_id)

human_groups                          -- teams
  id               uint (PK)
  name             string (unique)
  description      string

human_group_members
  group_id         uint FK → human_groups.id
  user_id          uint FK → users.id
  PK: (group_id, user_id)

agent_profile_acl                     -- LEGACY (dormant). Migrated into access_bindings
  ...                                 -- on startup; kept one release for rollback, then removed.
```

> **Migration.** On startup every `agent_profile_acl` row is copied into
> `access_bindings` as `(principal_type=user, resource_type=profile)`. The legacy
> table is left intact (read by nothing) so a rollback is safe; it will be dropped
> in a later release.

---

## Resource Hierarchy

```
agent          ← ephemeral running instance; id is STABLE: "<registration>-<localId>"
  ⮑ inherits   (re-registration / restart upserts the same id, so agent grants persist)
agent profile  ← logical role/type (e.g. "sales-manager"); stable named identity
system         ← global, via users.system_role_id
```

Grants attach at the **profile** or **agent** level. Agent ids are deterministic
and survive restarts, which is what makes per-agent ACLs durable. Registrations
are *not* an access boundary — they are auth credentials; multiple profiles can
share one registration.

---

## Teams (human groups)

A team groups users. Grant a team a role on a profile or agent, and every member
inherits it. Workflow:

1. **Create team** — `POST /groups`
2. **Give the team access** — grant it on a profile or agent (`principal_type:"group"`)
3. **Add members** — `POST /groups/{id}/members`

Removing a member, or deleting the team, immediately revokes the inherited access.

System roles are **per-user only** — a team cannot grant `admin` or any
`can_manage_*` system permission. Teams grant profile/agent access only.

---

## Access Resolution

Effective check for a user on a resource, for a given permission:

```
UserHasAgentPerm(user, agentId, perm):
  if user.system_role grants can_manage_users     → ALLOW   (admin bypass)
  principals = {user} ∪ {teams the user is a member of}
  if any binding(principals, agent  = agentId,            role ∋ perm) → ALLOW   (agent-level)
  if any binding(principals, profile = profileOf(agentId), role ∋ perm) → ALLOW   (inherited)
  → DENY

UserHasProfilePerm(user, profileName, perm):
  if user.system_role grants can_manage_users     → ALLOW
  principals = {user} ∪ {teams the user is a member of}
  if any binding(principals, profile = profileName, role ∋ perm) → ALLOW
  → DENY
```

Three layers stack **additively** — a team grant, a direct user grant, and a
profile-inherited grant are each sufficient on their own.

- **"Two agents in one profile, user sees only one"** → grant the *agent*, never
  the *profile*.
- **Admin bypass** — a system role with `can_manage_users` sees and manages
  everything, ignoring bindings.
- **Deny by default** — a user with no grants sees nothing; agents without a
  profile are not silently world-readable.

---

## Enforcement Points

| Endpoint | Check |
|---|---|
| `GET /agents` | filtered to (profiles the user can view) ∪ (agents granted directly) |
| `GET /agents/{id}` | `can_view_agents` on the agent → else **403** |
| `POST /agents/{id}/chat` | `can_chat_with_agents` on the agent → else 403 |
| `GET /agents/{id}/workspace`, `/sessions` | `can_view_agents` on the agent |
| `PUT /agents/{id}/workspace/locks` | `can_configure_agents` on the agent |
| `GET /me/agents` | profiles where the user can chat at the profile level **or** any agent within it |
| ACL & team management | `can_manage_profiles` |

> Note: agent endpoints return **403** (not 404) when access is denied.

---

## API

### Auth
```
POST /auth/login          { username, password } → { token, system_role }
GET  /auth/me             → { id, username, email, system_role, permissions[] }
```

### Users (require `can_manage_users`)
```
GET    /users
POST   /users             { username, password, email?, system_role_id? }
GET    /users/{id}
PUT    /users/{id}        { password?, email?, system_role_id? }
DELETE /users/{id}        -- also removes the user's bindings + team memberships
```

### Roles (require `can_manage_roles`)
```
GET    /roles
POST   /roles
GET    /roles/{id}
PUT    /roles/{id}                       -- system_default roles are immutable
DELETE /roles/{id}                       -- system_default roles cannot be deleted
POST   /roles/{id}/permissions           { permission }
DELETE /roles/{id}/permissions/{permission}
```

### Access bindings (require `can_manage_profiles`)
Grant body accepts `principal_type` (`user` | `group`) + `principal_id`; the
legacy `user_id` field is still accepted and treated as `principal_type=user`.

```
# Profile-level
GET    /agent-profiles/{name}/acl
POST   /agent-profiles/{name}/acl        { principal_type, principal_id, role_id }
DELETE /agent-profiles/{name}/acl/{ptype}/{pid}
DELETE /agent-profiles/{name}/acl/{pid}          -- legacy: deletes a user binding

# Agent-level (per-individual-agent grants)
GET    /agents/{id}/acl
POST   /agents/{id}/acl                  { principal_type, principal_id, role_id }
DELETE /agents/{id}/acl/{ptype}/{pid}
DELETE /agents/{id}/acl/{pid}                    -- legacy: deletes a user binding
```

`GET …/acl` returns, per grant:
```json
{ "principal_type": "group", "principal_id": 2, "principal_label": "Marketing",
  "role_id": 4, "role_name": "chatter" }
```

### Teams / human groups (require `can_manage_profiles`)
```
GET    /groups
POST   /groups                           { name, description? }
PUT    /groups/{id}                       { name?, description? }
DELETE /groups/{id}                       -- removes members + the team's bindings
GET    /groups/{id}/members
POST   /groups/{id}/members               { user_id }
DELETE /groups/{id}/members/{user_id}
```

### Per-principal access view (require `can_manage_profiles`)
The reverse of `…/acl` — "what can this principal reach", in one place.
```
GET /users/{id}/access     → [ { resource_type, resource_id, role_name, source } ]
GET /groups/{id}/access    → [ { resource_type, resource_id, role_name, source } ]
```
For a user, `source` is `"direct"` or `"team:<name>"` (so inherited grants are
distinguishable from direct ones).

### Self-service (any authenticated user)
```
GET  /me/agents           -- agents/profiles the caller can chat (honors agent-level grants)
GET  /me/tokens           -- the caller's personal access tokens (PATs)
POST /me/tokens           { name, expiresInDays? } → token shown once
DELETE /me/tokens/{id}
```

---

## UI

The control-plane UI exposes this through dedicated tabs:

- **Agents** — each agent has an **Access** button (admins only) → grant a user or
  team a role on that single agent.
- **Profiles** — each profile has an **Access** button → grant on the whole profile.
- **Humans** — create/edit users, assign system roles. Each row has an **Access**
  button showing everything that user can reach (direct **and** via teams, tagged
  `team:<name>`).
- **Teams** — create teams, manage members, and view a team's grants.
- **Roles** — create/edit roles and toggle permissions; built-in roles are read-only.
- **Access Tokens** — the caller's PATs.

The Access grant modal's grantee picker lists both **Users** and **Teams**.

---

## Authentication & Tokens

Two distinct auth paths:

- **Users** authenticate with a **JWT** (`POST /auth/login`, HS256, `JWT_SECRET`)
  or a **personal access token (PAT)** for CLI/scripts. Both resolve to the same
  user, so a PAT is scoped exactly like the user's session — `/agents`,
  `/me/agents`, chat, etc. all return the same access-filtered results.
- **Agents / clutch** authenticate with **registration tokens** and **agent
  tokens** (the `withAgent` middleware: register, heartbeat, logs, a2a,
  agent-chat). These are independent of user ACLs and are unaffected by the
  user-facing access model.

JWT claims:
```json
{ "uid": 42, "username": "alice", "system_role": "admin", "exp": ..., "iat": ... }
```

---

## A2A Delegation Scope

Agent-to-agent delegation (`canSendToTarget`) is intentionally **profile-scoped**:
delegation addresses a *role/profile* ("ask a sales-manager"), not a specific
instance. A user delegating is checked with `UserHasProfilePerm(...)`.

Consequence: a user who only has an **agent-level** chat grant can chat that agent
directly, and it appears in `/me/agents`, but A2A delegation to its profile is
still denied unless they also hold the profile-level grant. With single-agent
profiles there is no practical difference.

---

## Worked Example

### Resources
```
Agent Profiles → Agents
  cto                → openclaw-tech-team-cto-id
  techlead           → openclaw-tech-team-techlead-id
  sales-manager      → picoclaw-sales-sales-manager-id
  marketing-manager  → picoclaw-marketing-marketing-manager-id
  ceo                → picoclaw-ceo-office-ceo-id
```

### Principals & grants
```
Teams:
  Engineering  → grant: chatter on AGENT openclaw-tech-team-cto-id   (per-agent)
  Marketing    → grant: chatter on PROFILE marketing-manager         (per-profile)

Users:
  admin     → system role: admin
  surjeet   → member of Engineering
  bhuvnesh  → member of Marketing
  amit      → member of Marketing
  tushar    → no team, no grants
```

### What each user sees
| User | `/agents` | Chat allowed | `/me/agents` | Admin APIs |
|---|---|---|---|---|
| admin | all 5 | all 5 | all (profile perms) | yes |
| surjeet | `cto` only | `cto` | `[cto]` | 403 |
| bhuvnesh / amit | `marketing-manager` only | `marketing-manager` | `[marketing-manager]` | 403 |
| tushar | none | none | `[]` | 403 |

`admin/users/2/access` for surjeet returns:
`agent openclaw-tech-team-cto-id · chatter · via team:Engineering`.

---

## CLI

The `control-plane` binary manages the first admin directly against the DB (no
HTTP/auth needed):

```bash
control-plane createadmin --username admin --password <pass>   # always assigns admin
control-plane listadmins
control-plane deleteadmin --username <name>
```

All other users, teams, roles and grants are managed via the UI/API after login.

---

## Custom Roles

Admins can define custom profile-scoped roles, then grant them to a user or team
on a profile or agent. Example — a cron manager that can't chat or configure:

```
role "cron-manager" (profile-scoped):
  can_view_agents, can_view_crons, can_add_crons, can_edit_crons,
  can_delete_crons, can_trigger_crons

grant: POST /agents/{id}/acl { principal_type:"group", principal_id:<team>, role_id:<cron-manager> }
```
