package llm

import (
	"slices"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// toolCallAccumulator assembles tool calls that arrive in pieces across stream
// events. Both OpenAI protocols deliver a call's identifier, name, and argument
// JSON in separate chunks keyed by position, so they share this.
type toolCallAccumulator struct {
	byIndex map[int]*partialToolCall
}

type partialToolCall struct {
	id        string
	name      string
	arguments string
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{byIndex: make(map[int]*partialToolCall)}
}

// add merges one chunk. Empty fields leave what was already accumulated alone;
// argument text appends, because that is the field providers split.
func (a *toolCallAccumulator) add(index int, id, name, argumentsDelta string) {
	entry, ok := a.byIndex[index]
	if !ok {
		entry = &partialToolCall{}
		a.byIndex[index] = entry
	}
	if id != "" {
		entry.id = id
	}
	if name != "" {
		entry.name = name
	}
	entry.arguments += argumentsDelta
}

// toolCalls returns the assembled calls in position order. An entry that never
// received an identifier is dropped: it cannot be answered with a tool result,
// so sending it on would produce a turn the next request could not represent.
func (a *toolCallAccumulator) toolCalls() []cllm.ToolCall {
	if len(a.byIndex) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(a.byIndex))
	for index := range a.byIndex {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)

	var out []cllm.ToolCall
	for _, index := range indexes {
		entry := a.byIndex[index]
		if entry.id == "" {
			continue
		}
		out = append(out, cllm.ToolCall{
			ID:        entry.id,
			Name:      entry.name,
			Arguments: entry.arguments,
		})
	}
	return out
}

// imageFollowUpPreamble introduces images that a tool returned but its protocol
// could not attach to the result. Without it the images arrive as a user turn
// with no explanation, which reads as the user having sent them.
const imageFollowUpPreamble = "Images returned by the tool call above:"

// dataURL renders an image part the way the OpenAI protocols take one.
func dataURL(image cllm.ContentPart) string {
	return "data:" + image.MediaType + ";base64," + image.Data
}
