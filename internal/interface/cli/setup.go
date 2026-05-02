package cli

import "buildmax/internal/execution/agentrun"

// setupResult holds everything returned by setupAgentAndSession.
type setupResult struct {
	Runtime     *agentrun.Runtime
	SessionsDir string
	CWD         string
	ModelName   string
}

// setupAgentAndSession preserves the existing CLI setup seam while delegating to app/agentrun.
func setupAgentAndSession(sessionID string, modelSelector string) (setupResult, error) {
	rt, err := agentrun.Open(agentrun.OpenInput{
		SessionID:     sessionID,
		ModelSelector: modelSelector,
		EnableMCP:     true,
	})
	if err != nil {
		return setupResult{}, err
	}
	return setupResult{
		Runtime:     rt,
		SessionsDir: rt.SessionsDir,
		CWD:         rt.Workspace,
		ModelName:   rt.ModelName,
	}, nil
}
