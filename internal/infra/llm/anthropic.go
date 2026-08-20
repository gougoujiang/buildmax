package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// anthropicAdapter speaks the Anthropic Messages API.
//
// This protocol is stricter than the OpenAI ones about the shape of a
// conversation: the system prompt is a top-level parameter, tool results are
// blocks inside a user message, and an unpaired tool call is rejected outright.
// Canonical history is permissive by design and can be trimmed or compacted
// mid-conversation, so repairing it into a valid request is this adapter's job
// rather than a constraint pushed back onto core/llm.
type anthropicAdapter struct {
	client    anthropic.Client
	model     string
	maxTokens int
}

func newAnthropicAdapter(cfg Config) (*anthropicAdapter, error) {
	if cfg.Model == "" {
		return nil, errors.New("anthropic provider needs a model")
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		// Retries are this package's job, and doing them in two places would
		// multiply the wait a caller sees on a rate limit.
		option.WithMaxRetries(0),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	return &anthropicAdapter{
		client:    anthropic.NewClient(opts...),
		model:     cfg.Model,
		maxTokens: maxTokensOrDefault(cfg.MaxTokens),
	}, nil
}

func (a *anthropicAdapter) name() string { return config.LLMProviderAnthropic }

func (a *anthropicAdapter) buildParams(messages []cllm.Message, tools []cllm.ToolDef) (anthropic.MessageNewParams, error) {
	system, converted, err := anthropicMessages(messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(a.maxTokens),
		Messages:  converted,
		Tools:     anthropicTools(tools),
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	return params, nil
}

// anthropicMessages converts canonical history into the system parameter and a
// valid message sequence.
//
// The repairs it makes, and why each is needed:
//
//   - system messages are lifted out, because this protocol has no system role;
//   - a run of tool results becomes one user message, because the protocol
//     requires every result for a turn to arrive together;
//   - a tool call with no result, and a result with no call, are dropped,
//     because trimming and compaction can remove either half of a pair;
//   - messages before the first user message are dropped, because a
//     conversation cannot open with model output;
//   - empty text is never emitted, because an empty block is rejected.
func anthropicMessages(messages []cllm.Message) (string, []anthropic.MessageParam, error) {
	var systemParts []string
	body := make([]cllm.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
			continue
		}
		body = append(body, m)
	}

	start := -1
	for i, m := range body {
		if m.Role == "user" {
			start = i
			break
		}
	}
	if start < 0 {
		return "", nil, errors.New("anthropic messages need at least one user message")
	}
	body = body[start:]

	// Which halves of a tool-call pair actually survived into this history.
	calledIDs := make(map[string]struct{})
	answeredIDs := make(map[string]struct{})
	for _, m := range body {
		for _, tc := range m.ToolCalls {
			calledIDs[tc.ID] = struct{}{}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			answeredIDs[m.ToolCallID] = struct{}{}
		}
	}

	var out []anthropic.MessageParam
	for i := 0; i < len(body); {
		m := body[i]
		switch m.Role {
		case "tool":
			// Consume the whole run of results and emit them as one user turn.
			var blocks []anthropic.ContentBlockParamUnion
			for ; i < len(body) && body[i].Role == "tool"; i++ {
				result := body[i]
				if _, called := calledIDs[result.ToolCallID]; !called {
					continue
				}
				blocks = append(blocks, anthropic.NewToolResultBlock(result.ToolCallID, result.Content, false))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				if _, answered := answeredIDs[tc.ID]; !answered {
					continue
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, anthropicToolInput(tc.Arguments), tc.Name))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewAssistantMessage(blocks...))
			}
			i++
		default:
			if m.Content != "" {
				out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}
			i++
		}
	}
	if len(out) == 0 {
		return "", nil, errors.New("anthropic messages are empty after normalization")
	}
	return strings.Join(systemParts, "\n\n"), out, nil
}

// anthropicToolInput turns recorded argument JSON back into a value the request
// encodes as an object. Arguments that are absent or not valid JSON become an
// empty object: the model's own call is what BuildMax recorded, and refusing to
// replay the turn would strand the conversation.
func anthropicToolInput(arguments string) any {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return map[string]any{}
	}
	return json.RawMessage(trimmed)
}

// anthropicTools converts tool definitions, preserving any schema keywords
// beyond properties and required rather than silently narrowing a schema the
// tool author wrote.
func anthropicTools(tools []cllm.ToolDef) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			InputSchema: anthropicInputSchema(t.Parameters),
		}
		if t.Description != "" {
			tool.Description = anthropic.String(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

func anthropicInputSchema(parameters any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{}
	if parameters == nil {
		return schema
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return schema
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return schema
	}
	for key, value := range decoded {
		switch key {
		case "type":
			// Always "object" for a tool input schema; the field is implicit.
		case "properties":
			schema.Properties = value
		case "required":
			if names, ok := value.([]any); ok {
				for _, name := range names {
					if s, ok := name.(string); ok {
						schema.Required = append(schema.Required, s)
					}
				}
			}
		default:
			if schema.ExtraFields == nil {
				schema.ExtraFields = map[string]any{}
			}
			schema.ExtraFields[key] = value
		}
	}
	return schema
}

// anthropicError converts an SDK failure into the neutral error the shared
// retry and classification logic reads.
func anthropicError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return &apiError{status: apiErr.StatusCode, message: string(apiErr.Type()), err: err}
	}
	return &apiError{err: err}
}

// anthropicContent reads response blocks into canonical content and tool calls.
func anthropicContent(blocks []anthropic.ContentBlockUnion) (string, []cllm.ToolCall) {
	var text strings.Builder
	var toolCalls []cllm.ToolCall
	for _, block := range blocks {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			text.WriteString(variant.Text)
		case anthropic.ToolUseBlock:
			toolCalls = append(toolCalls, cllm.ToolCall{
				ID:        variant.ID,
				Name:      variant.Name,
				Arguments: string(variant.Input),
			})
		}
	}
	return text.String(), toolCalls
}

// anthropicUsage maps reported tokens onto the canonical shape. This protocol
// reports no total, so one is computed: metering reads TotalTokens, and leaving
// it zero would report a call that cost nothing.
func anthropicUsage(usage anthropic.Usage) cllm.Usage {
	prompt := int(usage.InputTokens)
	completion := int(usage.OutputTokens)
	return cllm.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
}

func (a *anthropicAdapter) blocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	params, err := a.buildParams(messages, tools)
	if err != nil {
		return "", nil, cllm.Usage{}, &requestError{err: err}
	}
	message, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("messages: %w", anthropicError(err))
	}
	content, toolCalls := anthropicContent(message.Content)
	return content, toolCalls, anthropicUsage(message.Usage), nil
}

func (a *anthropicAdapter) streaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	params, err := a.buildParams(messages, tools)
	if err != nil {
		return "", nil, cllm.Usage{}, &requestError{err: err}
	}
	stream := a.client.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	// The SDK assembles blocks, tool inputs, and usage from the event stream, so
	// the finished message is read the same way a blocking one is.
	var message anthropic.Message
	var delivered strings.Builder
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return delivered.String(), nil, cllm.Usage{}, fmt.Errorf("stream accumulate: %w", err)
		}
		deltaEvent, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
		if !ok {
			continue
		}
		textDelta, ok := deltaEvent.Delta.AsAny().(anthropic.TextDelta)
		if !ok || textDelta.Text == "" {
			continue
		}
		delivered.WriteString(textDelta.Text)
		if onDelta != nil {
			onDelta(textDelta.Text)
		}
	}
	if err := stream.Err(); err != nil {
		return delivered.String(), nil, cllm.Usage{}, fmt.Errorf("messages stream: %w", anthropicError(err))
	}
	content, toolCalls := anthropicContent(message.Content)
	return content, toolCalls, anthropicUsage(message.Usage), nil
}
