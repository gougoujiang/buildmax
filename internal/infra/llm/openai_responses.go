package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"

	openai "github.com/sashabaranov/go-openai"
)

// openAIResponsesAdapter speaks OpenAI's own Responses API.
//
// It runs stateless: the full input is sent on every call, and neither
// previous_response_id nor server-side storage is used. BuildMax owns history,
// trimming, compaction, and session persistence; server-side conversation state
// would compete with all four.
type openAIResponsesAdapter struct {
	client    *openai.Client
	model     string
	maxTokens int
	reasoning bool
}

func newOpenAIResponsesAdapter(cfg Config) *openAIResponsesAdapter {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}
	if cfg.HTTPClient != nil {
		clientConfig.HTTPClient = cfg.HTTPClient
	}
	if clientConfig.HTTPClient == nil {
		clientConfig.HTTPClient = http.DefaultClient
	}
	return &openAIResponsesAdapter{
		client:    openai.NewClientWithConfig(clientConfig),
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		reasoning: cfg.Reasoning,
	}
}

func (a *openAIResponsesAdapter) name() string { return config.LLMProviderOpenAI }

// buildRequest turns canonical history into Responses input items.
//
// System messages become top-level instructions, because this protocol has no
// system role. Everything else keeps its order.
func (a *openAIResponsesAdapter) buildRequest(messages []cllm.Message, tools []cllm.ToolDef) openai.CreateResponseRequest {
	var instructions []string
	input := make([]any, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			if m.Content != "" {
				instructions = append(instructions, m.Content)
			}
		case "tool":
			input = append(input, openai.ResponseFunctionCallOutput{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		case "assistant":
			// Reasoning precedes the output it produced, and is replayed
			// verbatim: the items are encrypted, so they are carried, not read.
			input = append(input, responsesReasoningItems(m.ProviderState)...)
			if m.Content != "" {
				input = append(input, openai.ResponseInputMessage{Role: "assistant", Content: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input = append(input, openai.ResponseOutputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				})
			}
		default:
			input = append(input, openai.ResponseInputMessage{Role: m.Role, Content: m.Content})
		}
	}

	responseTools := make([]openai.ResponseTool, 0, len(tools))
	for _, t := range tools {
		responseTools = append(responseTools, openai.NewResponseFunctionTool(openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		}))
	}

	req := openai.CreateResponseRequest{
		Model:           a.model,
		Input:           input,
		Instructions:    strings.Join(instructions, "\n\n"),
		Tools:           responseTools,
		MaxOutputTokens: a.maxTokens,
	}
	// The default is server-side storage of every response. BuildMax keeps the
	// conversation itself, so opt out rather than leave copies behind.
	store := false
	req.Store = &store
	if a.reasoning {
		// Reasoning items are only returned in a form that can be replayed when
		// asked for explicitly. Without storage there is no previous_response_id
		// to point at, so the encrypted content has to come back in the response
		// or continuity is lost between tool calls.
		req.Include = []openai.ResponseInclude{openai.ResponseIncludeReasoningEncryptedContent}
	}
	return req
}

// responsesReasoning collects the reasoning items from one response.
//
// The items are captured as the raw values the protocol sent. Decoding them
// into the library's item type and re-encoding would drop the encrypted content,
// because that field is not modelled — and the encrypted content is the whole
// point of keeping them.
func responsesReasoning(items []any) *cllm.ProviderState {
	var reasoning []any
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["type"] == "reasoning" {
			reasoning = append(reasoning, item)
		}
	}
	if len(reasoning) == 0 {
		return nil
	}
	data, err := json.Marshal(reasoning)
	if err != nil {
		return nil
	}
	return &cllm.ProviderState{Protocol: config.LLMProviderOpenAI, Data: data}
}

// responsesReasoningItems rebuilds the input items to replay. State from another
// protocol is ignored: this one would reject it, and the turn is still valid
// without it.
func responsesReasoningItems(state *cllm.ProviderState) []any {
	if !state.Belongs(config.LLMProviderOpenAI) {
		return nil
	}
	var items []any
	if err := json.Unmarshal(state.Data, &items); err != nil {
		return nil
	}
	return items
}

// responsesOutput reads the protocol's output items into canonical content and
// tool calls. Items arrive as untyped JSON, so each is re-decoded into the
// library's item shape rather than type-asserted.
func responsesOutput(items []any) (string, []cllm.ToolCall, error) {
	var content strings.Builder
	var toolCalls []cllm.ToolCall
	for _, raw := range items {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return "", nil, fmt.Errorf("encode output item: %w", err)
		}
		var item openai.ResponseOutputItem
		if err := json.Unmarshal(encoded, &item); err != nil {
			return "", nil, fmt.Errorf("decode output item: %w", err)
		}
		text, call := responsesItem(item)
		content.WriteString(text)
		if call != nil {
			toolCalls = append(toolCalls, *call)
		}
	}
	return content.String(), toolCalls, nil
}

// responsesItem maps one output item to the text it contributes and the tool
// call it carries, if any.
//
// A function call is identified by its call_id, not its item id: call_id is
// what a later function_call_output must reference, so it is the identifier
// that has to survive into the canonical history.
func responsesItem(item openai.ResponseOutputItem) (string, *cllm.ToolCall) {
	switch item.Type {
	case "message":
		var text strings.Builder
		for _, part := range item.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
		return text.String(), nil
	case "function_call":
		if item.CallID == "" {
			return "", nil
		}
		return "", &cllm.ToolCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments}
	default:
		return "", nil
	}
}

func responsesUsage(usage *openai.ResponseUsage) cllm.Usage {
	if usage == nil {
		return cllm.Usage{}
	}
	return cllm.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func (a *openAIResponsesAdapter) blocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (cllm.Completion, error) {
	resp, err := a.client.CreateResponse(ctx, a.buildRequest(messages, tools))
	if err != nil {
		return cllm.Completion{}, fmt.Errorf("create response: %w", openAIAPIError(err))
	}
	if resp.Error != nil {
		return cllm.Completion{}, &apiError{message: resp.Error.Message, err: errors.New(resp.Error.Code)}
	}
	content, toolCalls, err := responsesOutput(resp.Output)
	if err != nil {
		return cllm.Completion{}, err
	}
	return cllm.Completion{
		Content:       content,
		ToolCalls:     toolCalls,
		Usage:         responsesUsage(resp.Usage),
		ProviderState: responsesReasoning(resp.Output),
	}, nil
}

func (a *openAIResponsesAdapter) streaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (cllm.Completion, error) {
	stream, err := a.client.CreateResponseStream(ctx, a.buildRequest(messages, tools))
	if err != nil {
		return cllm.Completion{}, fmt.Errorf("create response stream: %w", openAIAPIError(err))
	}
	defer func() { _ = stream.Close() }()

	var (
		fullContent strings.Builder
		toolCalls   []cllm.ToolCall
		usage       cllm.Usage
		reasoning   *cllm.ProviderState
	)
	for {
		event, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return cllm.Completion{Content: fullContent.String()}, fmt.Errorf("stream recv: %w", openAIAPIError(err))
		}
		switch event.Type {
		case openai.ResponseStreamEventOutputTextDelta:
			if event.Delta == "" {
				continue
			}
			fullContent.WriteString(event.Delta)
			if onDelta != nil {
				onDelta(event.Delta)
			}
		case openai.ResponseStreamEventOutputItemDone:
			// A completed item carries assembled arguments, so the argument
			// deltas that preceded it need no accumulation of their own.
			if event.Item == nil {
				continue
			}
			if _, call := responsesItem(*event.Item); call != nil {
				toolCalls = append(toolCalls, *call)
			}
		case openai.ResponseStreamEventCompleted:
			if event.Response == nil {
				continue
			}
			usage = responsesUsage(event.Response.Usage)
			// Reasoning is read from the finished response rather than the
			// per-item events, whose decoded form drops the encrypted content.
			reasoning = responsesReasoning(event.Response.Output)
			if len(toolCalls) == 0 {
				// A provider that omitted per-item events still reports the
				// finished output here.
				_, calls, decodeErr := responsesOutput(event.Response.Output)
				if decodeErr == nil {
					toolCalls = calls
				}
			}
		case openai.ResponseStreamEventFailed, openai.ResponseStreamEventIncomplete:
			if event.Response != nil && event.Response.Error != nil {
				return cllm.Completion{Content: fullContent.String()},
					&apiError{message: event.Response.Error.Message, err: errors.New(event.Response.Error.Code)}
			}
			return cllm.Completion{Content: fullContent.String()}, &apiError{message: string(event.Type)}
		}
	}
	return cllm.Completion{
		Content:       fullContent.String(),
		ToolCalls:     toolCalls,
		Usage:         usage,
		ProviderState: reasoning,
	}, nil
}
