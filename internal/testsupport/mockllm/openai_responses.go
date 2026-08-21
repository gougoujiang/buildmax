package mockllm

import (
	"encoding/json"
	"net/http"
)

// OpenAI Responses. The adapter reads output items rather than a message, and
// identifies a call by call_id, which is what a later function_call_output
// references.

func writeOpenAIResponses(w http.ResponseWriter, step Step, model string, stream bool) {
	if stream {
		writeOpenAIResponsesStream(w, step, model)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responsesBody(step, model))
}

// writeOpenAIResponsesStream sends text deltas, one done event per tool call,
// and the finished response. A real provider also sends argument deltas; the
// adapter reads assembled arguments off the done event, so scripting them
// would add noise no assertion could see.
func writeOpenAIResponsesStream(w http.ResponseWriter, step Step, model string) {
	stream := newSSE(w)
	sequence := 0
	send := func(payload map[string]any) {
		payload["sequence_number"] = sequence
		sequence++
		name, _ := payload["type"].(string)
		stream.send(name, payload)
	}
	for _, part := range splitInTwo(step.Text) {
		send(map[string]any{"type": "response.output_text.delta", "item_id": "msg_mockllm", "delta": part})
	}
	for i, item := range responsesOutputItems(step) {
		record, _ := item.(map[string]any)
		if record["type"] != "function_call" {
			continue
		}
		send(map[string]any{"type": "response.output_item.done", "output_index": i, "item": record})
	}
	send(map[string]any{"type": "response.completed", "response": responsesBody(step, model)})
	stream.done()
}

// responsesBody is the finished response. The streaming path sends the same
// object inside its completed event, which is where the adapter reads usage.
func responsesBody(step Step, model string) map[string]any {
	usage := step.Usage
	if usage == nil {
		usage = &Usage{}
	}
	return map[string]any{
		"id":         "resp_mockllm",
		"object":     "response",
		"created_at": 0,
		"status":     "completed",
		"model":      model,
		"output":     responsesOutputItems(step),
		"usage": map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  usage.PromptTokens + usage.CompletionTokens,
		},
	}
}

// responsesOutputItems renders the turn as this protocol's output items.
func responsesOutputItems(step Step) []any {
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
	return output
}
