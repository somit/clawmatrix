package runners

import (
	"net/http"
	"os"
	"strings"
)

// Agent is the runner-local view of a registered agent.
type Agent struct {
	ID           string
	Command      string
	SessionsPath string
}

// Discovery holds configuration for an agent discovered from a runner config.
type Discovery struct {
	ID        string
	Group     string
	Default   bool
	Workspace string
	Subagents []string
}

// Config contains process-level runner settings.
type Config struct {
	Kind            string
	AgentCmd        string
	SessionsPath    string
	AgentGatewayURL string
}

// Runner handles runner-specific behaviour: command building, output parsing,
// and agent discovery.
type Runner interface {
	CommandArgs(agent Agent, msg, session string) []string
	UsesStdin() bool
	Env() []string
	PrepareSession(agent Agent, session string)
	ParseOutput(stdout, stderr string) (response, thinking string, usage map[string]any)
	ServeSessions(w http.ResponseWriter, r *http.Request, agent Agent) bool
	AgentCmd(localID string) string
	SessionsPath(localID string) string
	DiscoverAgents() []Discovery
}

// New returns the runner implementation for cfg.Kind.
func New(cfg Config) Runner {
	switch cfg.Kind {
	case "picoclaw":
		return &picoclawRunner{cfg: cfg}
	case "openclaw":
		return &openclawRunner{cfg: cfg}
	case "google-adk":
		return &googleAdkRunner{cfg: cfg}
	default:
		return &genericRunner{cfg: cfg}
	}
}

func splitFields(s string) []string {
	return strings.Fields(s)
}

func envAll() []string {
	return append([]string(nil), os.Environ()...)
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// genericRunner is a minimal fallback: passes message via stdin, returns stdout as-is.
type genericRunner struct {
	cfg Config
}

func (g *genericRunner) CommandArgs(agent Agent, _, session string) []string {
	return append(splitFields(agent.Command), "--session", session)
}
func (g *genericRunner) UsesStdin() bool                  { return true }
func (g *genericRunner) Env() []string                    { return envAll() }
func (g *genericRunner) PrepareSession(_ Agent, _ string) {}
func (g *genericRunner) DiscoverAgents() []Discovery      { return nil }
func (g *genericRunner) AgentCmd(_ string) string         { return g.cfg.AgentCmd }
func (g *genericRunner) SessionsPath(_ string) string     { return g.cfg.SessionsPath }
func (g *genericRunner) ServeSessions(http.ResponseWriter, *http.Request, Agent) bool {
	return false
}
func (g *genericRunner) ParseOutput(stdout, _ string) (string, string, map[string]any) {
	return trimSpace(stdout), "", nil
}
