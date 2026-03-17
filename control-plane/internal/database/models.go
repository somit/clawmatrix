package database

import "time"

type AcmeCache struct {
	Key       string    `gorm:"primaryKey"`
	Data      []byte    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type Registration struct {
	ID              uint   `gorm:"primaryKey"`
	Name            string `gorm:"uniqueIndex;not null"`
	Description     string `gorm:"type:text;not null;default:''"`
	TokenHash       string `gorm:"uniqueIndex;not null"`
	EgressAllowlist string `gorm:"type:text;not null;default:'[]'"` // JSON []string, supports wildcards
	TTLMinutes      int    `gorm:"not null;default:-1"`
	TotalRegistered int        `gorm:"not null;default:0"`
	Archived        bool       `gorm:"not null;default:false"`
	MonitorLastSeen *time.Time // last heartbeat from sniffer monitor
	Labels          string `gorm:"type:text;not null;default:'{}'"` // JSON map[string]string
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
