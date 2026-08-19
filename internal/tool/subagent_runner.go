package tool

import (
	"context"
	"fmt"
	"log/slog"

	coreagent "github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// defaultSubAgentMaxIter is the iteration cap for sub-agents.
// Lower than DefaultMaxIterations (200) because sub-agents are scoped, bounded tasks.
const defaultSubAgentMaxIter = 50

// SubAgentRunOpts configures one sub-agent invocation.
type SubAgentRunOpts struct {
	Tools        []llm.Tool
	SystemPrompt string
	Description  string
	MaxIter      int    // 0 = defaultSubAgentMaxIter
	Model        string // "" = use runner default client
}

// SubAgentRunner runs a sub-agent with the given options and prompt.
// It is implemented by agentapp and injected when building the Task tool so that
// tools do not depend on a concrete runner; tests can inject a mock.
type SubAgentRunner interface {
	RunSubAgent(ctx context.Context, opts SubAgentRunOpts, prompt string) (reply string, err error)
}

type defaultSubAgentRunner struct {
	client        llm.LLMClient
	policy        coreagent.ToolPolicy
	hooks         coreagent.HookRunner
	modelResolver func(string) (llm.LLMClient, error) // nil = always use client
}

// SubAgentRunnerOption configures a SubAgentRunner.
type SubAgentRunnerOption func(*defaultSubAgentRunner)

// WithSubAgentHooks attaches a parent hook runner so subagent runs honor the same
// PreToolUse / PostToolUse / lifecycle hooks as the parent agent. Nil disables hooks.
func WithSubAgentHooks(h coreagent.HookRunner) SubAgentRunnerOption {
	return func(r *defaultSubAgentRunner) { r.hooks = h }
}

// NewDefaultSubAgentRunner returns a SubAgentRunner backed by the given LLM client.
// policy is inherited from the parent agent run (nil = AllowAll).
// modelResolver looks up an LLM client by model name for agent types that specify a model;
// nil means always use client regardless of the model field.
func NewDefaultSubAgentRunner(llmClient llm.LLMClient, policy coreagent.ToolPolicy, modelResolver func(string) (llm.LLMClient, error), opts ...SubAgentRunnerOption) (SubAgentRunner, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("subagent runner: LLM client must not be nil")
	}
	r := &defaultSubAgentRunner{
		client:        llmClient,
		policy:        policy,
		modelResolver: modelResolver,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func (r *defaultSubAgentRunner) RunSubAgent(ctx context.Context, opts SubAgentRunOpts, prompt string) (string, error) {
	client := r.client
	if opts.Model != "" && r.modelResolver != nil {
		if c, err := r.modelResolver(opts.Model); err == nil {
			client = c
		} else {
			slog.Warn("subagent model not found, using default client", "model", opts.Model, "err", err)
		}
	}

	maxIter := opts.MaxIter
	if maxIter <= 0 {
		maxIter = defaultSubAgentMaxIter
	}

	sess := session.NewSession(opts.Description)
	registry := llm.NewToolRegistry()
	registry.AppendTools(opts.Tools...)
	if err := sess.Append(llm.Message{Role: "user", Content: prompt}); err != nil {
		return "", err
	}
	ctx = session.CtxWithSessionID(ctx, sess.ID)
	// Point durable state at the subagent's own session. The parent's store arrives on the
	// context, and leaving it in place would let a subagent overwrite the notes and task list
	// of the run that delegated to it. This session is discarded when the subagent returns.
	ctx = coreagent.CtxWithNoteStore(ctx, sess)

	// Fire SubagentStart so audit hooks can correlate the subagent run with
	// its parent. The decision is ignored — SubagentStart is advisory.
	if r.hooks != nil {
		r.hooks.Run(ctx, coreagent.HookInput{
			Event:      coreagent.HookSubagentStart,
			SessionID:  sess.ID,
			IsSubagent: true,
			AgentType:  opts.Description,
			Prompt:     prompt,
		})
	}

	reply, _, err := coreagent.RunLoop(ctx, coreagent.RunLoopOpts{
		LLMClient:    client,
		SystemPrompt: opts.SystemPrompt,
		ToolRegistry: registry,
		MaxIter:      maxIter,
		History:      sess,
		Policy:       r.policy,
		Hooks:        r.hooks,
		SessionID:    sess.ID,
		IsSubagent:   true,
		AgentType:    opts.Description,
	})
	if err != nil {
		return "", err
	}
	return reply, nil
}
