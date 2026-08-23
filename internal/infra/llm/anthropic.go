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
	client      anthropic.Client
	model       string
	maxTokens   int
	reasoning   string
	promptCache bool
	vision      bool
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
	opts = append(opts, option.WithHTTPClient(withBuildMaxUserAgent(cfg.HTTPClient, cfg.Surface)))
	return &anthropicAdapter{
		client:      anthropic.NewClient(opts...),
		model:       cfg.Model,
		maxTokens:   maxTokensOrDefault(cfg.MaxTokens),
		reasoning:   cfg.Reasoning,
		promptCache: cfg.PromptCache,
		vision:      cfg.Vision,
	}, nil
}

func (a *anthropicAdapter) name() string { return config.LLMProviderAnthropic }

func (a *anthropicAdapter) buildParams(messages []cllm.Message, tools []cllm.ToolDef) (anthropic.MessageNewParams, error) {
	system, converted, err := anthropicMessages(messages, a.vision)
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
		block := anthropic.TextBlockParam{Text: system}
		if a.promptCache {
			// Tools and the system prompt render before the messages and are
			// the same on every call in a run, so one breakpoint at the end of
			// the system prompt caches both. History is not cached here: a
			// second breakpoint is placed on the last block below, where it
			// covers whatever the turn actually ended with.
			block.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		params.System = []anthropic.TextBlockParam{block}
	}
	if a.promptCache {
		// The top-level marker attaches to the last cacheable block in the
		// request, so the next turn reads the whole prefix rather than only the
		// part before the conversation.
		params.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	if config.ReasoningEnabled(a.reasoning) {
		// Adaptive is the only supported mode on current models; a fixed token
		// budget is rejected by them. Display is omitted rather than summarized
		// because BuildMax needs the signature for multi-turn continuity, not
		// the reasoning text: the agent's transcript is what the user reads, and
		// thinking narration in it would be indistinguishable from an answer.
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{
				Display: anthropic.ThinkingConfigAdaptiveDisplayOmitted,
			},
		}
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(a.reasoning),
		}
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
func anthropicMessages(messages []cllm.Message, vision bool) (string, []anthropic.MessageParam, error) {
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
				blocks = append(blocks, anthropicToolResult(result, vision))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
		case "assistant":
			// Thinking comes first in the turn that produced it, and is replayed
			// exactly as received: the signature covers the block, so editing it
			// is worse than omitting it.
			blocks := anthropicThinkingBlocks(m.ProviderState)
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			thinkingOnly := len(blocks)
			for _, tc := range m.ToolCalls {
				if _, answered := answeredIDs[tc.ID]; !answered {
					continue
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, anthropicToolInput(tc.Arguments), tc.Name))
			}
			// A turn of nothing but thinking says nothing, and this protocol
			// rejects an assistant message with no visible content.
			if m.Content == "" && len(blocks) == thinkingOnly {
				i++
				continue
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewAssistantMessage(blocks...))
			}
			i++
		default:
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			if vision {
				for _, image := range m.Images() {
					blocks = append(blocks, anthropic.NewImageBlockBase64(image.MediaType, image.Data))
				}
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
			i++
		}
	}
	if len(out) == 0 {
		return "", nil, errors.New("anthropic messages are empty after normalization")
	}
	return strings.Join(systemParts, "\n\n"), out, nil
}

// anthropicToolResult renders one tool result. This protocol takes image blocks
// inside a tool_result, so a returned image reaches the model where it belongs
// rather than as a separate turn.
func anthropicToolResult(result cllm.Message, vision bool) anthropic.ContentBlockParamUnion {
	images := result.Images()
	if !vision || len(images) == 0 {
		return anthropic.NewToolResultBlock(result.ToolCallID, result.Content, false)
	}
	content := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(images)+1)
	if result.Content != "" {
		content = append(content, anthropic.ToolResultBlockParamContentUnion{
			OfText: &anthropic.TextBlockParam{Text: result.Content},
		})
	}
	for _, image := range images {
		content = append(content, anthropic.ToolResultBlockParamContentUnion{
			OfImage: &anthropic.ImageBlockParam{
				Source: anthropic.ImageBlockParamSourceUnion{
					OfBase64: &anthropic.Base64ImageSourceParam{
						Data:      image.Data,
						MediaType: anthropic.Base64ImageSourceMediaType(image.MediaType),
					},
				},
			},
		})
	}
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: result.ToolCallID,
			Content:   content,
		},
	}
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

// anthropicThinking is one recorded reasoning block. The fields are the whole
// of what the two block types carry, so recording them is lossless — and the
// signature is opaque by contract, never inspected, only returned.
type anthropicThinking struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

// anthropicProviderState records the turn's reasoning blocks, or nil when the
// model produced none. An absent state and an empty one are the same fact here,
// and nil is the one that costs nothing to store.
func anthropicProviderState(blocks []anthropic.ContentBlockUnion) *cllm.ProviderState {
	var recorded []anthropicThinking
	for _, block := range blocks {
		switch variant := block.AsAny().(type) {
		case anthropic.ThinkingBlock:
			recorded = append(recorded, anthropicThinking{
				Type:      "thinking",
				Thinking:  variant.Thinking,
				Signature: variant.Signature,
			})
		case anthropic.RedactedThinkingBlock:
			recorded = append(recorded, anthropicThinking{Type: "redacted_thinking", Data: variant.Data})
		}
	}
	if len(recorded) == 0 {
		return nil
	}
	data, err := json.Marshal(recorded)
	if err != nil {
		// State that cannot be recorded is dropped rather than half-written:
		// a partial signature would be rejected on the next turn.
		return nil
	}
	return &cllm.ProviderState{Protocol: config.LLMProviderAnthropic, Data: data}
}

// anthropicThinkingBlocks rebuilds the blocks to replay. State from another
// protocol is ignored: this one would reject it, and the turn is still valid
// without it.
func anthropicThinkingBlocks(state *cllm.ProviderState) []anthropic.ContentBlockParamUnion {
	if !state.Belongs(config.LLMProviderAnthropic) {
		return nil
	}
	var recorded []anthropicThinking
	if err := json.Unmarshal(state.Data, &recorded); err != nil {
		return nil
	}
	var blocks []anthropic.ContentBlockParamUnion
	for _, block := range recorded {
		switch block.Type {
		case "thinking":
			blocks = append(blocks, anthropic.NewThinkingBlock(block.Signature, block.Thinking))
		case "redacted_thinking":
			blocks = append(blocks, anthropic.NewRedactedThinkingBlock(block.Data))
		}
	}
	return blocks
}

// anthropicContent reads response blocks into canonical content and tool calls.
// Thinking is not content: it is recorded as provider state and never joins the
// text a user reads.
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
	// Cached input is reported apart from InputTokens here, so the prompt total
	// has to add it back: a cached call would otherwise look like it read
	// almost no prompt at all.
	cacheRead := int(usage.CacheReadInputTokens)
	cacheWrite := int(usage.CacheCreationInputTokens)
	prompt := int(usage.InputTokens) + cacheRead + cacheWrite
	completion := int(usage.OutputTokens)
	return cllm.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
	}
}

func (a *anthropicAdapter) blocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (cllm.Completion, error) {
	params, err := a.buildParams(messages, tools)
	if err != nil {
		return cllm.Completion{}, &requestError{err: err}
	}
	message, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return cllm.Completion{}, fmt.Errorf("messages: %w", anthropicError(err))
	}
	content, toolCalls := anthropicContent(message.Content)
	return cllm.Completion{
		Content:       content,
		ToolCalls:     toolCalls,
		Usage:         anthropicUsage(message.Usage),
		ProviderState: anthropicProviderState(message.Content),
	}, nil
}

func (a *anthropicAdapter) streaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (cllm.Completion, error) {
	params, err := a.buildParams(messages, tools)
	if err != nil {
		return cllm.Completion{}, &requestError{err: err}
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
			return cllm.Completion{Content: delivered.String()}, fmt.Errorf("stream accumulate: %w", err)
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
		return cllm.Completion{Content: delivered.String()}, fmt.Errorf("messages stream: %w", anthropicError(err))
	}
	content, toolCalls := anthropicContent(message.Content)
	return cllm.Completion{
		Content:       content,
		ToolCalls:     toolCalls,
		Usage:         anthropicUsage(message.Usage),
		ProviderState: anthropicProviderState(message.Content),
	}, nil
}
