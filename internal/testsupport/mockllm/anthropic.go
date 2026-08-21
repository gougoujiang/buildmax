package mockllm

import (
	"encoding/json"
	"net/http"
)

// Anthropic Messages. Text and tool use are content blocks of one message, and
// tool input is an object rather than the JSON string the OpenAI protocols
// carry.

func writeAnthropic(w http.ResponseWriter, step Step, model string, stream bool) {
	if stream {
		writeAnthropicStream(w, step, model)
		return
	}
	content := make([]any, 0, len(step.ToolCalls)+1)
	if step.Text != "" {
		content = append(content, map[string]any{"type": "text", "text": step.Text})
	}
	for _, call := range step.ToolCalls {
		input := call.Args
		if input == nil {
			input = map[string]any{}
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    call.ID,
			"name":  call.Name,
			"input": input,
		})
	}
	stopReason := "end_turn"
	if len(step.ToolCalls) > 0 {
		stopReason = "tool_use"
	}
	usage := step.Usage
	if usage == nil {
		usage = &Usage{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":          "msg_mockllm",
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"content":     content,
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
		},
	})
}

// writeAnthropicStream sends the same message as the event sequence the SDK
// accumulates: a start, one open/delta/stop trio per content block, then the
// stop reason and the turn's output tokens. Blocks are indexed in order with no
// gaps, which the accumulator enforces.
func writeAnthropicStream(w http.ResponseWriter, step Step, model string) {
	usage := step.Usage
	if usage == nil {
		usage = &Usage{}
	}
	stream := newSSE(w)
	stream.send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_mockllm", "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil,
			"usage": map[string]any{"input_tokens": usage.PromptTokens, "output_tokens": 0},
		},
	})

	index := 0
	open := func(block map[string]any) {
		stream.send("content_block_start", map[string]any{
			"type": "content_block_start", "index": index, "content_block": block,
		})
	}
	closeBlock := func() {
		stream.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		index++
	}

	if step.Text != "" {
		open(map[string]any{"type": "text", "text": ""})
		for _, part := range splitInTwo(step.Text) {
			stream.send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": index,
				"delta": map[string]any{"type": "text_delta", "text": part},
			})
		}
		closeBlock()
	}
	for _, call := range step.ToolCalls {
		open(map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}})
		// The whole argument object arrives as one partial: a scenario scripts
		// what the model asked for, not how many fragments it took to say it.
		stream.send("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": argumentsJSON(call)},
		})
		closeBlock()
	}

	stopReason := "end_turn"
	if len(step.ToolCalls) > 0 {
		stopReason = "tool_use"
	}
	stream.send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": usage.CompletionTokens},
	})
	stream.send("message_stop", map[string]any{"type": "message_stop"})
}
