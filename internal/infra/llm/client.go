// Package llm implements the core/llm.LLMClient contract over the LLM wire
// protocols BuildMax speaks: OpenAI Chat Completions, OpenAI Responses, and
// Anthropic Messages.
//
// One package, one entry point. Config.Provider selects an adapter; everything
// a real network call needs to survive — the per-call timeout, the retry loop,
// and error classification — is shared, so the three protocols cannot drift
// apart on the parts a caller depends on.
//
// Mirrors the design in docs/design/llm-provider-adapters.md.
package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// Config holds the settings for one LLM client.
type Config struct {
	// Provider is the wire protocol to speak. Empty means
	// config.LLMProviderOpenAICompatible.
	Provider      string
	APIKey        string
	BaseURL       string
	Model         string
	ContextWindow int           // 0 = look up, then fall back to config.DefaultContextWindow
	MaxTokens     int           // 0 = the adapter's own default
	CallTimeout   time.Duration // 0 = use DefaultCallTimeoutSecs
	// Reasoning asks the protocol for reasoning state and replays it on later
	// turns. It does nothing on a protocol that has none.
	Reasoning bool
	// HTTPClient overrides the transport. Tests use it; production leaves it nil.
	HTTPClient *http.Client
}

// adapter speaks one wire protocol. It performs a single attempt and reports
// failures as *apiError where it can classify them, so the shared retry and
// classification logic never has to know which library produced the error.
type adapter interface {
	// name is the provider value this adapter implements.
	name() string
	blocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (cllm.Completion, error)
	streaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (cllm.Completion, error)
}

// LLMClient calls one model through one wire protocol and holds the
// configuration for those calls.
type LLMClient struct {
	adapter       adapter
	contextWindow int
	callTimeout   time.Duration
}

// ContextWindow returns the configured context window size (0 = no windowing).
func (c *LLMClient) ContextWindow() int { return c.contextWindow }

// Provider returns the wire protocol this client speaks. Diagnostics and the
// trace use it to say which protocol served a call.
func (c *LLMClient) Provider() string { return c.adapter.name() }

// NewClient builds an LLM client for the configured wire protocol.
//
// An unknown provider is an error rather than a fallback: a model that cannot
// be reached the way it was configured must fail at selection, not send its
// prompt somewhere the operator did not name.
func NewClient(cfg Config) (*LLMClient, error) {
	cw := cfg.ContextWindow
	if cw == 0 {
		cw = lookupContextWindow(cfg.Model)
	}
	if cw == 0 {
		cw = config.DefaultContextWindow
	}
	callTimeout := cfg.CallTimeout
	if callTimeout == 0 {
		callTimeout = time.Duration(config.DefaultCallTimeoutSecs) * time.Second
	}

	var (
		impl adapter
		err  error
	)
	switch cfg.Provider {
	case "", config.LLMProviderOpenAICompatible:
		impl = newOpenAIChatAdapter(cfg)
	case config.LLMProviderOpenAI:
		impl = newOpenAIResponsesAdapter(cfg)
	case config.LLMProviderAnthropic:
		impl, err = newAnthropicAdapter(cfg)
	default:
		return nil, fmt.Errorf("unknown llm provider %q: use %s, %s, or %s",
			cfg.Provider,
			config.LLMProviderOpenAICompatible,
			config.LLMProviderOpenAI,
			config.LLMProviderAnthropic)
	}
	if err != nil {
		return nil, err
	}

	return &LLMClient{
		adapter:       impl,
		contextWindow: cw,
		callTimeout:   callTimeout,
	}, nil
}

// ChatCompletionBlocking sends messages and tool definitions, returns assistant content, any tool calls, and usage.
// Retries on rate-limit and transient server errors up to maxRetryAttempts times.
// Errors are wrapped with a human-readable classification before being returned.
func (c *LLMClient) ChatCompletionBlocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (completion cllm.Completion, err error) {
	for attempt := range maxRetryAttempts {
		if attempt > 0 {
			logRetry(attempt, err)
			if sleepErr := sleepWithContext(ctx, retryBackoff[attempt-1]); sleepErr != nil {
				return cllm.Completion{}, wrapLLMError(sleepErr)
			}
		}
		callCtx, cancel := c.withCallTimeout(ctx)
		completion, err = c.adapter.blocking(callCtx, messages, tools)
		cancel()
		if err == nil || !isRetryableError(err) {
			break
		}
	}
	if err != nil {
		err = wrapLLMError(err)
	}
	return
}

// ChatCompletionStreaming sends messages and tool definitions, streams content deltas via onDelta,
// and returns full content, any tool calls, and usage (when the provider reports it).
// Retries on transient errors only when no delta has been emitted yet, to avoid duplicate output.
// Errors are wrapped with a human-readable classification before being returned.
// If onDelta is nil, it is not called.
func (c *LLMClient) ChatCompletionStreaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(delta string)) (completion cllm.Completion, err error) {
	for attempt := range maxRetryAttempts {
		if attempt > 0 {
			logRetry(attempt, err)
			if sleepErr := sleepWithContext(ctx, retryBackoff[attempt-1]); sleepErr != nil {
				return cllm.Completion{}, wrapLLMError(sleepErr)
			}
		}
		var deltaEmitted bool
		guardedDelta := func(delta string) {
			deltaEmitted = true
			if onDelta != nil {
				onDelta(delta)
			}
		}
		callCtx, cancel := c.withCallTimeout(ctx)
		completion, err = c.adapter.streaming(callCtx, messages, tools, guardedDelta)
		cancel()
		if err == nil || !isRetryableError(err) || deltaEmitted {
			break
		}
	}
	if err != nil {
		err = wrapLLMError(err)
	}
	return
}

// withCallTimeout bounds one attempt. The returned cancel is always safe to
// call, so the caller does not branch on whether a timeout is configured.
func (c *LLMClient) withCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.callTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.callTimeout)
}

// maxTokensOrDefault returns the configured output cap, substituting the
// built-in default when a protocol requires the field and the operator set none.
func maxTokensOrDefault(configured int) int {
	if configured > 0 {
		return configured
	}
	return config.DefaultMaxTokens
}

// Ensure *LLMClient implements cllm.LLMClient.
var _ cllm.LLMClient = (*LLMClient)(nil)
