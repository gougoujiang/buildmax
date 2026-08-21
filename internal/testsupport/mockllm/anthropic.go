package mockllm

import (
	"encoding/json"
	"net/http"
)

// Anthropic Messages. Text and tool use are content blocks of one message, and
// tool input is an object rather than the JSON string the OpenAI protocols
// carry.

func writeAnthropic(w http.ResponseWriter, step Step, model string) {
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
