# User Management & Permissions

## Overview

ClawMatrix uses a Role-Based Access Control (RBAC) system.
Permissions are fixed constants defined in code. Roles are dynamic and stored in the database — default roles are seeded on startup, and admins can create custom roles.

---

## Permissions

Permissions are fixed strings defined in code. They are grouped by scope.

### System-scoped (global)
| Permission | Description |
|---|---|
| `can_manage_users` | Create, edit, delete users |
| `can_manage_roles` | Create, edit, delete roles |
| `can_manage_registrations` | Create, edit, delete agent registrations |
| `can_manage_profiles` | Create, edit, delete agent profiles |
| `can_manage_connections` | Manage agent-to-agent connections |
| `can_view_logs` | View request logs |
| `can_view_audit` | View audit trail |
| `can_view_metrics` | View metrics and stats |

### Profile-scoped (per agent profile)
| Permission | Description |
|---|---|
| `can_view_agents` | See agents and their details |
| `can_chat_with_agents` | Use agent chat |
| `can_configure_agents` | Kill agents, change status/config |
| `can_view_crons` | View cron jobs and executions |
| `can_trigger_crons` | Manually trigger a cron job |
| `can_add_crons` | Create new cron jobs |
| `can_edit_crons` | Edit existing cron jobs |
| `can_delete_crons` | Delete cron jobs |

---

## Data Model

```
users
  id               uint (PK)
  username         string (unique)
  password_hash    string
  system_role_id   uint FK → roles.id (nullable — no system role = member only)
  created_at       time
  updated_at       time

roles
  id               uint (PK)
  name             string (unique)
  description      string
  scope            enum: system | profile
  system_default   bool  -- seeded roles, cannot be deleted
  created_at       time
  updated_at       time

role_permissions
  role_id          uint FK → roles.id
  permission       string
  PK: (role_id, permission)

agent_profile_acl
  profile_name     string FK → agent_profiles.name
  user_id          uint FK → users.id
  role_id          uint FK → roles.id  (must be profile-scoped role)
  PK: (profile_name, user_id)
```

---

## Default Roles (seeded)

### System-scoped

| Role | Permissions |
|---|---|
| `admin` | all system permissions |

### Profile-scoped

| Role | Permissions |
|---|---|
| `owner` | all profile permissions |
| `operator` | can_view_agents, can_chat_with_agents, can_configure_agents, can_view_crons, can_trigger_crons, can_add_crons, can_edit_crons, can_delete_crons |
| `chatter` | can_view_agents, can_chat_with_agents, can_view_crons, can_trigger_crons |
| `viewer` | can_view_agents, can_view_crons |

Default roles have `system_default = true` and cannot be deleted (only custom roles can be removed).

---

## Access Control Logic

### System permissions
- User has `system_role_id` → load role → check `role_permissions`
- If no system role → no system permissions
- First user created is automatically assigned `admin` system role

### Profile permissions
- `GET /agents` → returns only profiles where user has `can_view_agents`
- `GET /agents/{id}` → **404** if user lacks `can_view_agents` (not 403 — do not reveal existence)
- All other profile actions → check `agent_profile_acl` for the relevant permission

### Admin bypass
- Users with a system role containing `can_manage_users` can see and manage all profiles regardless of `agent_profile_acl`
- `admin` system role implicitly bypasses all profile-level ACL checks

---

## Scoping: Why Agent Profile (not Registration)

- **Registration** = auth credential (token). Multiple profiles can share one registration.
- **Agent Profile** = logical role/type (e.g. "content-writer", "devops"). Stable named identity.
- **Agent** = ephemeral running instance. Comes and goes.

ACL lives on Agent Profile because:
- One registration can serve multiple teams via different profiles
- Agents are ephemeral — managing per-instance ACL is impractical
- Profile is the stable, human-meaningful boundary

---

## Concrete Example

### Setup

```
Registrations:
  marketing-reg    ← marketing agents authenticate here
  engineering-reg  ← engineering agents authenticate here

Agent Profiles:
  content-writer      → marketing-reg
  social-media        → marketing-reg
  seo-specialist      → marketing-reg
  campaign-manager    → marketing-reg
  backend-dev         → engineering-reg
  frontend-dev        → engineering-reg
  devops              → engineering-reg
  qa-engineer         → engineering-reg
```

### Users

| User | System Role | Notes |
|---|---|---|
| frank | admin | CTO — sees everything, bypasses all ACL |
| alice | — | Marketing manager |
| bob | — | Content writer |
| carol | — | Social media |
| dave | — | SEO |
| eve | — | Campaigns |

### system_grants (via system roles)

```
alice → (none — manages profiles via agent_profile_acl only)
frank → admin role → all permissions
```

> Note: alice can be given `can_manage_registrations` via a custom system role
> if she needs to manage the marketing registration.

### agent_profile_acl

| Profile | User | Role |
|---|---|---|
| content-writer | alice | operator |
| content-writer | bob | chatter |
| social-media | alice | operator |
| social-media | carol | chatter |
| seo-specialist | alice | operator |
| seo-specialist | dave | chatter |
| campaign-manager | alice | operator |
| campaign-manager | eve | chatter |

Frank (admin) sees all 8 profiles — no ACL rows needed.

### What each user sees

| User | Visible agents | Can chat | Can configure | Can manage crons |
|---|---|---|---|---|
| frank | all 8 | all 8 | all 8 | all 8 |
| alice | 4 marketing | 4 marketing | 4 marketing | 4 marketing |
| bob | content-writer | content-writer | — | trigger only |
| carol | social-media | social-media | — | trigger only |
| dave | seo-specialist | seo-specialist | — | trigger only |
| eve | campaign-manager | campaign-manager | — | trigger only |

---

## Custom Roles

Admins can create custom roles. Example:

```
role: "cron-manager" (profile-scoped)
  can_view_agents
  can_view_crons
  can_add_crons
  can_edit_crons
  can_delete_crons
  can_trigger_crons
  -- no can_chat_with_agents, no can_configure_agents
```

Assign to a user per profile:
```
agent_profile_acl: (campaign-manager, alice, "cron-manager")
```

---

## API

### Auth
```
POST /auth/login          { username, password } → { token, expires_at }
POST /auth/logout         (invalidates token)
GET  /auth/me             (current user info + permissions)
```

### Users (requires can_manage_users)
```
GET    /users
POST   /users
GET    /users/{id}
PUT    /users/{id}
DELETE /users/{id}
```

### Roles (requires can_manage_roles)
```
GET    /roles
POST   /roles
GET    /roles/{id}
PUT    /roles/{id}
DELETE /roles/{id}          (system_default roles cannot be deleted)
POST   /roles/{id}/permissions
DELETE /roles/{id}/permissions/{permission}
```

### Profile ACL (requires can_manage_profiles or admin)
```
GET    /agent-profiles/{name}/acl
POST   /agent-profiles/{name}/acl        { user_id, role_id }
DELETE /agent-profiles/{name}/acl/{user_id}
```

---

## Auth Token

Session tokens stored in `user_sessions` table:
```
user_sessions
  token_hash   string (PK)
  user_id      uint FK → users.id
  expires_at   time
  created_at   time
```

Token is a random 32-byte hex string. Hash (SHA-256) stored in DB, raw token returned to client once at login.
Existing `ADMIN_TOKEN` env var continues to work as a superuser API key for backward compatibility (scripts, clutch).

---

## Migration / Bootstrap

- On first startup with no users: create default roles + prompt/env-based admin user creation
- `ADMIN_TOKEN` still works indefinitely as a bypass for backward compat
- Existing single-token auth (`withAdmin` middleware) becomes: check `ADMIN_TOKEN` first, then check user session
