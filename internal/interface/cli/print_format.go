package cli

import (
	"encoding/json"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// OutputFormat is the print-mode output format selected by --output.
type OutputFormat string

const (
	OutputText  OutputFormat = "text"
	OutputJSON  OutputFormat = "json"
	OutputJSONL OutputFormat = "jsonl"
)

// printResult is the stable envelope emitted at the end of json/jsonl runs.
// It is also embedded as the final `result` record in jsonl mode.
// Field naming follows the repo's snake_case persistence convention.
type printResult struct {
	Type      string `json:"type,omitempty"` // "result" for jsonl, omitted for json
	SessionID string `json:"session_id"`
	// TraceID and TracePath point at the durable trace this run wrote. A caller
	// otherwise cannot tell which file is theirs: a session holds one trace per
	// run, and picking the newest is a race against a concurrent run in the same
	// session. Both are empty when tracing is off or failed to start, which is
	// not the same fact as a run that recorded nothing, so they are always
	// present rather than omitted.
	TraceID      string         `json:"trace_id"`
	TracePath    string         `json:"trace_path"`
	Model        string         `json:"model"`
	Workspace    string         `json:"workspace"`
	Reply        string         `json:"reply"`
	ToolCalls    int            `json:"tool_calls"`
	DurationMS   int64          `json:"duration_ms"`
	Usage        printUsage     `json:"usage"`
	Context      printContext   `json:"context"`
	ExitCode     int            `json:"exit_code"`
	Error        *printErrorObj `json:"error"`
	PolicyDenied bool           `json:"policy_denied,omitempty"`
}

type printUsage struct {
	Prompt          int `json:"prompt"`
	Completion      int `json:"completion"`
	TotalPrompt     int `json:"total_prompt"`
	TotalCompletion int `json:"total_completion"`
	// Cache counts break the prompt counts above down; a consumer that adds
	// them to prompt counts the same tokens twice. Zero is what a provider
	// without cache reporting sends, not a proven miss.
	CacheRead       int `json:"cache_read"`
	CacheWrite      int `json:"cache_write"`
	TotalCacheRead  int `json:"total_cache_read"`
	TotalCacheWrite int `json:"total_cache_write"`
	// Cost is the session's estimated spend, absent when the model was
	// unpriced. Amounts are nano-units of Currency — one unit is 1e9 of them —
	// so a consumer sums them exactly. Incomplete says part of the session
	// could not be priced and the figure understates it.
	Cost           *cllm.Cost `json:"cost,omitempty"`
	CostIncomplete bool       `json:"cost_incomplete,omitempty"`
}

type printContext struct {
	Tokens int `json:"tokens"`
	Window int `json:"window"`
}

type printErrorObj struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// buildResultEnvelope converts a RunResult plus run metadata into the wire envelope.
func buildResultEnvelope(out agentapp.RunResult, exitCode int, runErr error, policyDenied bool, jsonl bool) printResult {
	env := printResult{
		SessionID:  out.SessionID,
		TraceID:    out.TraceID,
		TracePath:  out.TracePath,
		Model:      out.ModelName,
		Workspace:  out.Workspace,
		Reply:      out.Reply,
		ToolCalls:  out.ToolCalls,
		DurationMS: out.Duration.Milliseconds(),
		Usage: printUsage{
			Prompt:          out.PromptTokens,
			Completion:      out.CompletionTokens,
			TotalPrompt:     out.TotalPromptTokens,
			TotalCompletion: out.TotalCompletionTokens,
			CacheRead:       out.CacheReadTokens,
			CacheWrite:      out.CacheWriteTokens,
			TotalCacheRead:  out.TotalCacheReadTokens,
			TotalCacheWrite: out.TotalCacheWriteTokens,
			Cost:            out.Cost,
			CostIncomplete:  out.CostIncomplete,
		},
		Context:      printContext{Tokens: out.ContextTokens, Window: out.ContextWindow},
		ExitCode:     exitCode,
		PolicyDenied: policyDenied,
	}
	if jsonl {
		env.Type = "result"
	}
	if runErr != nil {
		env.Error = &printErrorObj{Kind: errorKindForExitCode(exitCode), Message: runErr.Error()}
	}
	return env
}

func errorKindForExitCode(code int) string {
	switch code {
	case ExitPolicyDenied:
		return "policy_denied"
	case ExitModelError:
		return "model_error"
	case ExitToolError:
		return "tool_error"
	case ExitUserCancelled:
		return "cancelled"
	case ExitUsage:
		return "usage"
	default:
		return "error"
	}
}

// classifyExit derives a process exit code from the agent run outcome.
//   - ctx cancellation (userCancelled true) → ExitUserCancelled
//   - non-nil runErr with any policy denial observed → ExitPolicyDenied
//   - non-nil runErr otherwise → ExitModelError
//   - nil runErr → ExitOK
func classifyExit(runErr error, policyDenied bool, userCancelled bool) int {
	if userCancelled {
		return ExitUserCancelled
	}
	if runErr == nil {
		return ExitOK
	}
	if policyDenied {
		return ExitPolicyDenied
	}
	return ExitModelError
}

// eventToJSON converts an agent.Event to a JSON line for jsonl output.
// Returns (nil, false) when the event should be suppressed (e.g. delta events
// when includeDeltas is false, or iter-start which is verbose noise).
func eventToJSON(ev agent.Event, includeDeltas bool) ([]byte, bool) {
	rec := map[string]any{}
	switch ev.Kind {
	case agent.EventIterStart:
		return nil, false
	case agent.EventLLMStart:
		rec["type"] = "llm_start"
		rec["iter"] = ev.Iter
		if ev.ContextWindow > 0 {
			rec["context_tokens"] = ev.ContextTokens
			rec["context_window"] = ev.ContextWindow
		}
	case agent.EventLLMDelta:
		if !includeDeltas {
			return nil, false
		}
		rec["type"] = "llm_delta"
		rec["content"] = ev.Content
	case agent.EventLLMEnd:
		rec["type"] = "llm_end"
		rec["iter"] = ev.Iter
		rec["has_tool_calls"] = ev.HasToolCalls
		rec["prompt_tokens"] = ev.PromptTokens
		rec["completion_tokens"] = ev.CompletionTokens
		rec["cache_read_tokens"] = ev.CacheReadTokens
		rec["cache_write_tokens"] = ev.CacheWriteTokens
	case agent.EventToolStart:
		rec["type"] = "tool_start"
		rec["tool"] = ev.ToolName
		rec["tool_call_id"] = ev.ToolCallID
		rec["args"] = ev.ToolArgs
	case agent.EventToolEnd:
		rec["type"] = "tool_end"
		rec["tool"] = ev.ToolName
		rec["tool_call_id"] = ev.ToolCallID
		rec["duration_ms"] = ev.ToolDuration.Milliseconds()
		rec["result"] = ev.ToolResult
	case agent.EventToolDenied:
		rec["type"] = "tool_denied"
		rec["tool"] = ev.ToolName
		rec["tool_call_id"] = ev.ToolCallID
		rec["reason"] = ev.DenyReason
	case agent.EventContextCompacted:
		rec["type"] = "context_compacted"
		rec["summarized"] = ev.Summarized
		rec["kept"] = ev.Kept
	case agent.EventRunEnd:
		rec["type"] = "run_end"
		rec["tool_calls"] = ev.Stats.ToolCalls
		rec["prompt_tokens"] = ev.Stats.PromptTokens
		rec["completion_tokens"] = ev.Stats.CompletionTokens
		rec["cache_read_tokens"] = ev.Stats.CacheReadTokens
		rec["cache_write_tokens"] = ev.Stats.CacheWriteTokens
		if ev.Err != nil {
			rec["error"] = ev.Err.Error()
		}
	default:
		return nil, false
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return nil, false
	}
	return b, true
}
