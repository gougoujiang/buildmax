package agentrun

import (
	"context"
	"fmt"

	coreagent "buildmax/internal/core/agent"
	"buildmax/internal/core/model"
	"buildmax/internal/session"
)

// SubAgentRunner runs a sub-agent invocation using a separate ephemeral session.
// It is primarily used by the Task tool to delegate a subtask to a sub-agent.
type SubAgentRunner interface {
	// RunSubAgent runs the sub-agent with the given tools and system prompt.
	// description becomes the ephemeral session title.
	RunSubAgent(ctx context.Context, tools []model.Tool, systemPrompt string, description string, prompt string) (reply string, err error)
}

type defaultSubAgentRunner struct {
	llmClient model.LLMClient
}

// NewDefaultSubAgentRunner returns the default implementation of SubAgentRunner
// backed by the same LLM client used by the main agent.
func NewDefaultSubAgentRunner(llmClient model.LLMClient) (SubAgentRunner, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("subagent runner: LLM client must not be nil")
	}
	return &defaultSubAgentRunner{llmClient: llmClient}, nil
}

func (r *defaultSubAgentRunner) RunSubAgent(ctx context.Context, tools []model.Tool, systemPrompt string, description string, prompt string) (string, error) {
	sess := session.NewSession(description)
	registry := model.NewToolRegistry()
	registry.AppendTools(tools...)
	sub := coreagent.NewAgent(r.llmClient, registry, coreagent.SystemPrompt(systemPrompt))
	ctx = session.CtxWithSessionID(ctx, sess.ID())
	reply, _, err := sub.Process(ctx, sess, prompt)
	if err != nil {
		return "", err
	}
	return reply, nil
}
