package agentapp

import (
	"context"
	"fmt"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// mcpCaller adapts MCPManager to the hook.MCPCaller interface so the MCP
// driver in infra/hook does not need to know about MCPManager (and infra/hook
// stays free of agentapp imports).
type mcpCaller struct {
	m *MCPManager
}

// CallMCPTool invokes the named tool on the named server via the registry
// owned by MCPManager. Returns an error when MCP is not configured or the
// registry is empty.
func (c *mcpCaller) CallMCPTool(ctx context.Context, server, tool string, input map[string]any) (string, error) {
	if c == nil || c.m == nil {
		return "", fmt.Errorf("mcp manager not initialized")
	}
	reg := c.m.Registry()
	if reg == nil {
		return "", fmt.Errorf("mcp registry not available")
	}
	return reg.CallMcp(ctx, server, tool, input)
}

// llmCaller adapts the AgentApp's LLMClientCache to hook.LLMCaller for the
// prompt driver. An empty model name falls back to the AgentApp default,
// so prompt hooks can omit "model:" and still work.
type llmCaller struct {
	cache        *LLMClientCache
	defaultModel func() string
}

// CompleteHookPrompt runs a single-turn completion. The returned string is
// the model's text reply (the prompt driver parses it as optional JSON).
func (c *llmCaller) CompleteHookPrompt(ctx context.Context, model, prompt string) (string, error) {
	if c == nil || c.cache == nil {
		return "", fmt.Errorf("llm cache not initialized")
	}
	name := model
	if name == "" && c.defaultModel != nil {
		name = c.defaultModel()
	}
	client, err := c.cache.Get(name)
	if err != nil {
		return "", fmt.Errorf("resolve hook model %q: %w", name, err)
	}
	msgs := []cllm.Message{{Role: "user", Content: prompt}}
	completion, err := client.ChatCompletionBlocking(ctx, msgs, nil)
	if err != nil {
		return "", err
	}
	return completion.Content, nil
}
