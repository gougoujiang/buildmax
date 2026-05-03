package agentrun

import (
	"context"
	"fmt"

	coreagent "buildmax/internal/core/agent"
	"buildmax/internal/core/model"
	tools "buildmax/internal/execution/builtintool"
	"buildmax/internal/session"
)

type defaultSubAgentRunner struct {
	llmClient model.LLMClient
}

var _ tools.SubAgentRunner = (*defaultSubAgentRunner)(nil)

// NewDefaultSubAgentRunner returns the default builtintool.SubAgentRunner
// backed by the same LLM client used by the main agent.
func NewDefaultSubAgentRunner(llmClient model.LLMClient) (tools.SubAgentRunner, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("subagent runner: LLM client must not be nil")
	}
	return &defaultSubAgentRunner{llmClient: llmClient}, nil
}

func (r *defaultSubAgentRunner) RunSubAgent(ctx context.Context, tools []model.Tool, systemPrompt string, description string, prompt string) (string, error) {
	sess := session.NewSession(description)
	registry := model.NewToolRegistry()
	registry.AppendTools(tools...)
	sub := coreagent.NewAgent(r.llmClient, registry, coreagent.WithSystemPrompt(systemPrompt))
	ctx = session.CtxWithSessionID(ctx, sess.ID)
	reply, _, err := sub.Process(ctx, sess, prompt)
	if err != nil {
		return "", err
	}
	return reply, nil
}
