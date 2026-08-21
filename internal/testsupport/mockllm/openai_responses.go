package mockllm

import (
	"encoding/json"
	"net/http"
)

// OpenAI Responses. The adapter reads output items rather than a message, and
// identifies a call by call_id, which is what a later function_call_output
// references.

func writeOpenAIResponses(w http.ResponseWriter, step Step, model string) {
	output := make([]any, 0, len(step.ToolCalls)+1)
	if step.Text != "" {
		output = append(output, map[string]any{
			"type":    "message",
			"id":      "msg_mockllm",
			"status":  "completed",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": step.Text}},
		})
	}
	for _, call := range step.ToolCalls {
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        "fc_" + call.ID,
			"call_id":   call.ID,
			"name":      call.Name,
			"arguments": argumentsJSON(call),
			"status":    "completed",
		})
	}
	usage := step.Usage
	if usage == nil {
		usage = &Usage{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         "resp_mockllm",
		"object":     "response",
		"created_at": 0,
		"status":     "completed",
		"model":      model,
		"output":     output,
		"usage": map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  usage.PromptTokens + usage.CompletionTokens,
		},
	})
}
