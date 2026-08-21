package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// OpenAI Chat Completions. This is the default protocol for a model entry that
// names none, so it is the one most suites run.

type chatToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type chatMessage struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func writeOpenAIChat(w http.ResponseWriter, step Step, model string, stream bool) {
	if stream {
		writeOpenAIChatStream(w, step, model)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "mockllm",
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       chatMessage{Role: "assistant", Content: step.Text, ToolCalls: chatCalls(step)},
			"finish_reason": chatFinishReason(step),
		}},
		"usage": chatUsageOf(step),
	})
}

// writeOpenAIChatStream sends the same reply as deltas. Text arrives in two
// chunks so a suite that asserts on streamed output sees more than one write,
// which is where an accumulator bug shows up.
func writeOpenAIChatStream(w http.ResponseWriter, step Step, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	send := func(payload map[string]any) {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", encoded)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta chatMessage, finish any) map[string]any {
		choice := map[string]any{"index": 0, "delta": delta}
		if finish != nil {
			choice["finish_reason"] = finish
		}
		return map[string]any{
			"id": "mockllm", "object": "chat.completion.chunk", "created": 0, "model": model,
			"choices": []any{choice},
		}
	}

	send(chunk(chatMessage{Role: "assistant"}, nil))
	for _, part := range splitInTwo(step.Text) {
		send(chunk(chatMessage{Content: part}, nil))
	}
	for i, call := range chatCalls(step) {
		index := i
		call.Index = &index
		send(chunk(chatMessage{ToolCalls: []chatToolCall{call}}, nil))
	}
	final := chunk(chatMessage{}, chatFinishReason(step))
	final["usage"] = chatUsageOf(step)
	send(final)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func chatCalls(step Step) []chatToolCall {
	if len(step.ToolCalls) == 0 {
		return nil
	}
	out := make([]chatToolCall, 0, len(step.ToolCalls))
	for _, call := range step.ToolCalls {
		out = append(out, chatToolCall{
			ID:       call.ID,
			Type:     "function",
			Function: chatFunction{Name: call.Name, Arguments: argumentsJSON(call)},
		})
	}
	return out
}

func chatFinishReason(step Step) string {
	if len(step.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

func chatUsageOf(step Step) chatUsage {
	usage := step.Usage
	if usage == nil {
		usage = &Usage{}
	}
	return chatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.PromptTokens + usage.CompletionTokens,
	}
}

// splitInTwo cuts text roughly in half, and returns nothing for empty text so
// a tool-only turn streams no content deltas. It splits on runes: half a
// multi-byte character would reach the client as a replacement character.
func splitInTwo(text string) []string {
	runes := []rune(text)
	switch {
	case len(runes) == 0:
		return nil
	case len(runes) < 2:
		return []string{text}
	}
	half := len(runes) / 2
	return []string{string(runes[:half]), string(runes[half:])}
}
