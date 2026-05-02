// Package llm provides LLM client implementations (OpenRouter/OpenAI-compatible).
package llm

import "buildmax/internal/core/model"

type (
	LLMCaller  = model.LLMCaller
	StreamSink = model.StreamSink
)
