// Conversation tool: get_current_date. Returns the current date in YYYY-MM-DD format.
package conversation

import (
	"context"
	"time"

	"buildmax/internal/core/model"
)

// GetCurrentDate returns the current date in YYYY-MM-DD format.
// It implements Tool for the conversation (Tier 1) loop.
type GetCurrentDate struct{}

// Name returns the tool name for the LLM.
func (GetCurrentDate) Name() string { return ToolNameGetCurrentDate }

// Description returns a short description for the LLM.
func (GetCurrentDate) Description() string {
	return "Returns the current date in YYYY-MM-DD format."
}

// Parameters returns the OpenAI-style JSON schema (no arguments).
func (GetCurrentDate) Parameters() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// Execute returns the current date in YYYY-MM-DD format.
func (GetCurrentDate) Execute(ctx context.Context, args map[string]any) (string, error) {
	return time.Now().Format("2006-01-02"), nil
}

// Ensure GetCurrentDate implements model.Tool.
var _ model.Tool = GetCurrentDate{}
