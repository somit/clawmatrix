package clutch

import (
	"fmt"
	"regexp"
	"strings"
)

// picoclawRunner handles picoclaw-specific subprocess execution.
type picoclawRunner struct{}

func (p *picoclawRunner) CommandArgs(agent *RegisteredAgent, _ /*msg*/, session string) []string {
	if !strings.HasPrefix(session, "agent:") {
		session = "agent:main:" + session
	}
	return append(splitFields(agent.agentCmd), "--session", session)
}

func (p *picoclawRunner) UsesStdin() bool { return true }

func (p *picoclawRunner) Env() []string { return envAll() }

func (p *picoclawRunner) PrepareSession(_ *RegisteredAgent, _ string) {} // no-op

func (p *picoclawRunner) ParseOutput(stdout, stderr string) (string, string, map[string]any) {
	return parsePicoclawOutput(stdout), "", parsePicoclawUsage(stderr)
}

func (p *picoclawRunner) AgentCmd(_ string) string        { return AgentCmd }
func (p *picoclawRunner) SessionsPath(_ string) string    { return SessionsPath }
func (p *picoclawRunner) DiscoverAgents() []agentDiscovery { return nil }

// --- picoclaw output parsing ---

func parsePicoclawOutput(raw string) string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.Contains(line, "Interactive mode") {
			continue
		}
		if strings.TrimSpace(line) == "Goodbye!" {
			continue
		}
		if strings.HasPrefix(line, "🦞 ") {
			lines = append(lines, line[len("🦞 "):])
		} else if strings.TrimSpace(line) == "🦞" {
			continue
		} else {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var picoclawUsageIntKeys = []string{
	"iterations", "content_chars", "final_length",
	"prompt_tokens", "completion_tokens", "total_tokens",
}

func parsePicoclawUsage(stderr string) map[string]any {
	usage := map[string]any{}

	for _, key := range picoclawUsageIntKeys {
		re := regexp.MustCompile(key + `=(\d+)`)
		if m := re.FindStringSubmatch(stderr); m != nil {
			var v int
			fmt.Sscanf(m[1], "%d", &v)
			usage[key] = v
		}
	}

	toolRe := regexp.MustCompile(`tool=([a-zA-Z0-9_-]+)`)
	if matches := toolRe.FindAllStringSubmatch(stderr, -1); len(matches) > 0 {
		var tools []string
		for _, m := range matches {
			tools = append(tools, m[1])
		}
		usage["tool_calls"] = tools
	}

	if len(usage) == 0 {
		return nil
	}
	return usage
}
