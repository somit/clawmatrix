package clutch

import "clutch/internal/runners"

func newRunner() runners.Runner {
	return runners.New(runners.Config{
		Kind:            Runner,
		AgentCmd:        AgentCmd,
		SessionsPath:    SessionsPath,
		AgentGatewayURL: AgentGatewayURL,
	})
}

func InitRunner() {
	RunnerInstance = newRunner()
}

func getRunner() runners.Runner {
	if RunnerInstance == nil {
		InitRunner()
	}
	return RunnerInstance
}

func runnerAgent(agent *RegisteredAgent) runners.Agent {
	return runners.Agent{
		ID:           agent.id,
		Command:      agent.agentCmd,
		SessionsPath: agent.sessionsPath,
	}
}
