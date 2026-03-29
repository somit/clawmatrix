package clutch

import (
	"sync"
	"sync/atomic"
	"time"
)

// RegisteredAgent holds per-agent state for multi-agent support.
type RegisteredAgent struct {
	id                string // local id: "cto", "mflead"
	fullID            string // CP id: "cto-cto", "mflead-mflead"
	agentToken        string // per-agent auth token from CP
	registrationToken string // registration token (for heartbeats/config); empty for primary
	workspace         string // workspace root
	sessionsPath      string // sessions directory
	agentCmd          string // e.g. "openclaw agent --agent cto"
	isDefault         bool
	claudeSessions    map[string]string // maps clutch session ID → claude session_id for --resume
	mu                sync.Mutex        // serializes subprocess invocations (prevents concurrent session file conflicts)
}

// Accessor methods for use outside the package.
func (a *RegisteredAgent) LocalID() string   { return a.id }
func (a *RegisteredAgent) FullID() string    { return a.fullID }
func (a *RegisteredAgent) Workspace() string { return a.workspace }

var (
	CpURL   string
	CpToken string // registration token (primary)

	Allowlist atomic.Value // []string
	LastMod   atomic.Value // string (Last-Modified header)

	Stats struct {
		Allowed  atomic.Int64
		Blocked  atomic.Int64
		TotalMs  atomic.Int64
		ReqCount atomic.Int64
		MinMs    atomic.Int64
		MaxMs    atomic.Int64
	}

	PreferredAgentID    string
	PreferredAgentGroup string // role/group name (AGENT_GROUP env), defaults to agent id

	AgentCmd     string // e.g. "picoclaw agent"
	AgentTimeout time.Duration
	ListenAddr   string
	HostBaseURL  string // externally-reachable base URL (HOST_URL env), e.g. "http://tech-team:8080"

	LogAllowed bool
	LogBlocked bool

	Runner        string
	WorkspacePath string
	SessionsPath  string

	// Gateway forwarding for openclaw runner
	AgentGatewayURL   string // e.g. "http://localhost:18789"
	AgentGatewayToken string // Bearer token for gateway auth
	SnifferDisabled   bool

	LogBuf   []map[string]any
	LogBufMu sync.Mutex

	// Multi-agent state
	RegisteredAgents   []RegisteredAgent
	RegisteredAgentsMu sync.RWMutex
)
