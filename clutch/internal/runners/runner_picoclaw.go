package runners

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// picoclawRunner handles picoclaw-specific subprocess execution.
type picoclawRunner struct {
	cfg Config
}

func (p *picoclawRunner) CommandArgs(agent Agent, _ /*msg*/, session string) []string {
	if !strings.HasPrefix(session, "agent:") {
		session = "agent:main:" + session
	}
	return append(splitFields(agent.Command), "--session", session)
}

func (p *picoclawRunner) UsesStdin() bool { return true }

func (p *picoclawRunner) Env() []string { return envAll() }

func (p *picoclawRunner) PrepareSession(_ Agent, _ string) {} // no-op

func (p *picoclawRunner) ParseOutput(stdout, stderr string) (string, string, map[string]any) {
	return parsePicoclawOutput(stdout), "", parsePicoclawUsage(stderr)
}

func (p *picoclawRunner) AgentCmd(_ string) string     { return p.cfg.AgentCmd }
func (p *picoclawRunner) SessionsPath(_ string) string { return p.cfg.SessionsPath }
func (p *picoclawRunner) DiscoverAgents() []Discovery  { return nil }
func (p *picoclawRunner) ServeSessions(http.ResponseWriter, *http.Request, Agent) bool {
	return false
}

// --- picoclaw output parsing ---

func parsePicoclawOutput(raw string) string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = stripANSI(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "Interactive mode") {
			continue
		}
		if trimmed == "Goodbye!" {
			continue
		}
		if strings.Contains(trimmed, "wealthyclaw") && strings.Contains(trimmed, "personal ai") {
			continue
		}
		if strings.Contains(trimmed, "picoclaw") && strings.Contains(trimmed, "Personal AI Assistant") {
			continue
		}
		if strings.Contains(trimmed, `\|/`) {
			continue
		}
		if strings.HasPrefix(line, "🦞 ") {
			lines = append(lines, line[len("🦞 "):])
		} else if trimmed == "🦞" {
			continue
		} else {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
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
