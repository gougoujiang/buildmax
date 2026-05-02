// Package llm provides LLM client implementations (OpenRouter/OpenAI-compatible).
package llm

import "buildmax/internal/core/model"

type (
	Message  = model.Message
	ToolDef  = model.ToolDef
	ToolCall = model.ToolCall
	Usage    = model.Usage
)
