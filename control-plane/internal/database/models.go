package database

import "time"

// --- User management ---

type User struct {
	ID           uint    `gorm:"primaryKey"`
	Username     string  `gorm:"uniqueIndex;not null"`
	PasswordHash string  `gorm:"not null"`
	Email        *string `gorm:"uniqueIndex"`
	SystemRoleID *uint   `gorm:"index"`
	SystemRole   *Role   `gorm:"foreignKey:SystemRoleID"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Role struct {
	ID            uint             `gorm:"primaryKey"`
	Name          string           `gorm:"uniqueIndex;not null"`
	Description   string           `gorm:"type:text;not null;default:''"`
	Scope         string           `gorm:"not null"` // "system" | "profile"
	SystemDefault bool             `gorm:"not null;default:false"`
	Permissions   []RolePermission `gorm:"foreignKey:RoleID"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RolePermission struct {
	RoleID     uint   `gorm:"primaryKey"`
	Permission string `gorm:"primaryKey"`
}

// AgentProfileACL is the legacy per-profile, per-user grant table. It is kept
// (dormant) for one release so a migration to AccessBinding can be rolled back;
// new code reads and writes AccessBinding instead. Remove in a follow-up.
type AgentProfileACL struct {
	ProfileName string `gorm:"primaryKey"`
	UserID      uint   `gorm:"primaryKey"`
	RoleID      uint   `gorm:"not null;index"`
	Role        Role   `gorm:"foreignKey:RoleID"`
}

// Principal types for AccessBinding.PrincipalType.
const (
	PrincipalUser  = "user"
	PrincipalGroup = "group"
)

// Resource types for AccessBinding.ResourceType.
const (
	ResourceProfile = "profile" // ResourceID is the agent profile name
	ResourceAgent   = "agent"   // ResourceID is the agent id
)

// AccessBinding grants a principal (a user or a group) a role on a resource (an
// agent profile or a specific agent). It is the single source of truth for
// resource-scoped access. Effective access for a user is the union of their own
// bindings and the bindings of every group they belong to (see HumanGroupMember),
// with agent-level grants layered additively on top of profile-level grants.
type AccessBinding struct {
	ID            uint   `gorm:"primaryKey"`
	PrincipalType string `gorm:"uniqueIndex:idx_binding;not null;default:'user'"` // user | group
	PrincipalID   uint   `gorm:"uniqueIndex:idx_binding;not null"`
	ResourceType  string `gorm:"uniqueIndex:idx_binding;not null"`                // profile | agent
	ResourceID    string `gorm:"uniqueIndex:idx_binding;not null"`                // profile name | agent id
	RoleID        uint   `gorm:"not null;index"`
	Role          Role   `gorm:"foreignKey:RoleID"`
	CreatedAt     time.Time
}

// HumanGroup is a team of users. A group can be granted access (via AccessBinding
// with PrincipalType=group); every member inherits those grants.
type HumanGroup struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;not null"`
	Description string `gorm:"type:text;not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// HumanGroupMember links a user to a group.
type HumanGroupMember struct {
	GroupID  uint `gorm:"primaryKey"`
	UserID   uint `gorm:"primaryKey"`
	CreatedAt time.Time
}

type UserIdentity struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"uniqueIndex:idx_user_identity;not null"`
	Provider   string `gorm:"uniqueIndex:idx_user_identity;not null"` // "slack", "github", "oidc"
	ExternalID string `gorm:"uniqueIndex:idx_user_identity;not null"`
}

// UserToken is a personal access token (PAT) for non-interactive clients (CLI
// tools and scripts). Only the hash is stored; the raw token is shown once at creation.
type UserToken struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"index;not null"`
	Name       string `gorm:"not null;default:''"` // human label, e.g. "somit's laptop"
	TokenHash  string `gorm:"uniqueIndex;not null"`
	LastUsedAt *time.Time
	ExpiresAt  *time.Time // nil = never expires
	CreatedAt  time.Time
}

// Upload is metadata for a stored attachment blob. The bytes live in the
// configured storage backend (filesystem by default); only metadata is in the DB.
type Upload struct {
	ID        string `gorm:"primaryKey"` // opaque id, also the storage key
	UserID    uint   `gorm:"index;not null"`
	Name      string `gorm:"not null;default:''"`
	MimeType  string `gorm:"not null;default:''"`
	Size      int64  `gorm:"not null;default:0"`
	Backend   string `gorm:"not null;default:'fs'"`
	ExpiresAt *time.Time // nil = no expiry; set for TTL-based cleanup/GC
	CreatedAt time.Time
}

type AcmeCache struct {
	Key       string    `gorm:"primaryKey"`
	Data      []byte    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type Registration struct {
	ID              uint       `gorm:"primaryKey"`
	Name            string     `gorm:"uniqueIndex;not null"`
	Description     string     `gorm:"type:text;not null;default:''"`
	TokenHash       string     `gorm:"uniqueIndex;not null"`
	EgressAllowlist string     `gorm:"type:text;not null;default:'[]'"` // JSON []string, supports wildcards
	TTLMinutes      int        `gorm:"not null;default:-1"`
	TotalRegistered int        `gorm:"not null;default:0"`
	Archived        bool       `gorm:"not null;default:false"`
	MonitorLastSeen *time.Time // last heartbeat from sniffer monitor
	Labels          string     `gorm:"type:text;not null;default:'{}'"` // JSON map[string]string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Connection struct {
	ID        uint   `gorm:"primaryKey"`
	Source    string `gorm:"uniqueIndex:idx_connection;not null"` // agent profile name
	Target    string `gorm:"uniqueIndex:idx_connection;not null"` // agent profile name
	CreatedAt time.Time
}

type AgentProfile struct {
	ID               uint    `gorm:"primaryKey"`
	Name             string  `gorm:"uniqueIndex;not null"` // logical role/type
	Description      string  `gorm:"type:text;not null;default:''"`
	Registration     *string `gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // FK → Registration.Name (nullable)
	Image            string  `gorm:"not null;default:''"`                                  // container image (future)
	MaxCount         int     `gorm:"not null;default:0"`                                   // max agents from this template (0 = unlimited)
	TTLMinutes       int     `gorm:"not null;default:-1"`                                  // agent TTL (-1 = persistent)
	DeploymentConfig string  `gorm:"type:text;not null;default:'{}'"`                      // JSON — infra provisioning config
	Source           string  `gorm:"type:text;not null;default:''"`                        // automatic, manual
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Agent struct {
	ID           string  `gorm:"primaryKey"`
	AgentProfile *string `gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"` // FK → AgentProfile.Name
	Token        string  `gorm:"uniqueIndex;not null;default:''"`
	Status       string  `gorm:"not null;default:'healthy'"` // healthy, stale, kill
	KillReason   string
	Environment  string `gorm:"type:text;not null;default:'{}'"`
	Meta         string `gorm:"type:text;not null;default:'{}'"`
	Gateway      string `gorm:"type:text;not null;default:'{}'"`

	StatsAllowed  int64     `gorm:"not null;default:0"`
	StatsBlocked  int64     `gorm:"not null;default:0"`
	StatsAvgMs    int64     `gorm:"not null;default:0"`
	StatsMinMs    int64     `gorm:"not null;default:0"`
	StatsMaxMs    int64     `gorm:"not null;default:0"`
	StatsReqCount int64     `gorm:"not null;default:0"`
	RegisteredAt  time.Time `gorm:"not null"`
	LastHeartbeat time.Time `gorm:"not null"`
}

type A2ATask struct {
	ID              string `gorm:"primaryKey"`
	Kind            string `gorm:"not null;default:'task'"`
	ContextID       string `gorm:"index;not null"`
	RuntimeSession  string
	RuntimeRunner   string
	StatusState     string `gorm:"index;not null"`
	StatusMessage   string `gorm:"type:text;not null;default:'{}'"`
	StatusTimestamp time.Time
	History         string `gorm:"type:text;not null;default:'[]'"`
	Artifacts       string `gorm:"type:text;not null;default:'[]'"`
	Metadata        string `gorm:"type:text;not null;default:'{}'"`
	SourceAgentID   string `gorm:"index"`
	SourceProfile   string `gorm:"index"`
	TargetAgentID   string `gorm:"index"`
	TargetProfile   string `gorm:"index"`
	// Caller attribution — who initiated this ask (distinct from SourceProfile,
	// which overloads the agent-profile namespace). Set for every ask.
	CallerKind   string `gorm:"index"` // user | agent | registration
	CallerUserID uint   `gorm:"index"` // User.ID for user/PAT asks; 0 otherwise
	CallerName   string `gorm:"index"` // username, profile, or registration name (display)
	// ParentTaskID links this hop to the ask that triggered it (agent→agent
	// delegation). Empty = a root ask. Captured at delegation time by clutch.
	ParentTaskID string `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RequestLog struct {
	ID               uint   `gorm:"primaryKey"`
	RegistrationName string `gorm:"index;not null;default:''"`
	Domain           string `gorm:"index;not null;default:''"`
	Method           string `gorm:"not null;default:''"`
	Path             string
	Action           string    `gorm:"index;not null;default:''"` // allowed, blocked
	StatusCode       int       `gorm:"not null;default:0"`
	LatencyMs        int64     `gorm:"not null;default:0"`
	CreatedAt        time.Time `gorm:"index;not null"`
}

type AuditEvent struct {
	ID        uint      `gorm:"primaryKey"`
	EventType string    `gorm:"index;not null;default:''"`
	Data      string    `gorm:"type:text;not null;default:'{}'"` // JSON
	CreatedAt time.Time `gorm:"index;not null"`
}

type Metric struct {
	ID               uint      `gorm:"primaryKey"`
	AgentID          string    `gorm:"index;not null;default:''"`
	RegistrationName string    `gorm:"index;not null;default:''"`
	Allowed          int64     `gorm:"not null;default:0"`
	Blocked          int64     `gorm:"not null;default:0"`
	AvgMs            int64     `gorm:"not null;default:0"`
	MinMs            int64     `gorm:"not null;default:0"`
	MaxMs            int64     `gorm:"not null;default:0"`
	ReqCount         int64     `gorm:"not null;default:0"`
	CreatedAt        time.Time `gorm:"index;not null"`
}

type CronJob struct {
	ID               uint       `gorm:"primaryKey"`
	Name             string     `gorm:"not null"`
	Description      string     `gorm:"type:text;not null;default:''"`
	AgentProfileName string     `gorm:"index;not null;default:''"`
	RegistrationName string     `gorm:"index;not null;default:''"`
	Schedule         string     `gorm:"not null;default:''"`
	Timezone         string     `gorm:"not null;default:'UTC'"`
	RunAt            *time.Time // one-time execution; if set, Schedule is ignored
	Session          string     `gorm:"not null;default:''"`
	Message          string     `gorm:"type:text;not null"`
	Enabled          bool       `gorm:"not null;default:true"`
	NextRunAt        *time.Time
	LastRunAt        *time.Time
	LastStatus       string `gorm:"not null;default:''"`
	LastError        string `gorm:"type:text"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CronExecution struct {
	ID         uint      `gorm:"primaryKey"`
	CronJobID  uint      `gorm:"index;not null"`
	AgentID    string    `gorm:"index;not null;default:''"`
	Status     string    `gorm:"not null"`
	Error      string    `gorm:"type:text"`
	DurationMs int64     `gorm:"not null;default:0"`
	CreatedAt  time.Time `gorm:"index;not null"`
}
