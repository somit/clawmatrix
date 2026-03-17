package clutch

// agentDiscovery holds configuration for an agent discovered from a runner's config.
type agentDiscovery struct {
	ID        string
	Group     string
	Default   bool
	Workspace string
	Subagents []string // names of agents this agent may delegate to
}

// AgentRunner handles all runner-specific behaviour: command building, output parsing,
// and agent discovery. Add a new file (runner_<name>.go) to support a new runner.
type AgentRunner interface {
	// CommandArgs returns the full argv (executable + args) to spawn.
	CommandArgs(agent *RegisteredAgent, msg, session string) []string

	// UsesStdin reports whether the subprocess reads the message from stdin.
	UsesStdin() bool

	// Env returns the environment for the subprocess.
	Env() []string

	// PrepareSession performs any pre-run maintenance (e.g. session file repair).
	PrepareSession(agent *RegisteredAgent, session string)

	// ParseOutput extracts response, thinking, and usage from stdout/stderr.
	ParseOutput(stdout, stderr string) (response, thinking string, usage map[string]any)

	// AgentCmd returns the command string to store on a RegisteredAgent for localID.
	AgentCmd(localID string) string

	// SessionsPath returns the sessions directory for a given agent local ID.
	SessionsPath(localID string) string

	// DiscoverAgents reads runner-specific config and returns all configured agents.
	// Returns nil for single-agent runners (picoclaw, generic).
	DiscoverAgents() []agentDiscovery
}

// newRunner returns the AgentRunner for the current Runner global.
func newRunner() AgentRunner {
	switch Runner {
	case "picoclaw":
		return &picoclawRunner{}
	case "openclaw":
		return &openclawRunner{}
	default:
		return &genericRunner{}
	}
}

// genericRunner is a minimal fallback: passes message via stdin, returns stdout as-is.
type genericRunner struct{}

func (g *genericRunner) CommandArgs(agent *RegisteredAgent, _, session string) []string {
	return append(splitFields(agent.agentCmd), "--session", session)
}
func (g *genericRunner) UsesStdin() bool                                   { return true }
func (g *genericRunner) Env() []string                                     { return envAll() }
func (g *genericRunner) PrepareSession(_ *RegisteredAgent, _ string)       {}
func (g *genericRunner) DiscoverAgents() []agentDiscovery                  { return nil }
func (g *genericRunner) AgentCmd(_ string) string                          { return AgentCmd }
func (g *genericRunner) SessionsPath(_ string) string                      { return SessionsPath }
func (g *genericRunner) ParseOutput(stdout, _ string) (string, string, map[string]any) {
	return trimSpace(stdout), "", nil
}
