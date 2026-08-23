package trace

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/plugin"
)

// traceVersion is stamped on the run_start record so future readers can detect
// the schema generation.
const traceVersion = 1

// defaultMaxFieldBytes bounds each free-text field (content/args/result) so a
// single large tool result cannot bloat the trace file.
const defaultMaxFieldBytes = 4096

// Record is one line in a run's JSONL trace. Fields irrelevant to a given
// record type are omitted. Keys are snake_case per CLAUDE.md §6.1.
type Record struct {
	TS   string `json:"ts"`
	Type string `json:"type"`

	// run_start
	RunID      string `json:"run_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	Model      string `json:"model,omitempty"`
	IsSubagent bool   `json:"is_subagent,omitempty"`
	// ParentRunID is the immediate trace run that delegated this subagent.
	// Top-level runs leave it absent: an empty value would look like a broken
	// link rather than a run that has no parent.
	ParentRunID string `json:"parent_run_id,omitempty"`
	// ParentToolCallID is the tool call in the parent run that launched this
	// one, closing the walk parent run → tool call → child run.
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	TraceVersion     int    `json:"trace_version,omitempty"`

	// sandbox_boundary
	//
	// Sandboxed is a pointer so an unsandboxed run records "sandboxed": false
	// rather than omitting the field. A boundary that goes unreported is
	// indistinguishable from one that was never resolved, and a reader
	// resolving that silence in the run's favor would credit a run with
	// protection it did not have.
	Sandboxed   *bool    `json:"sandboxed,omitempty"`
	SandboxMode string   `json:"sandbox_mode,omitempty"`
	Backend     string   `json:"backend,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Downgraded  bool     `json:"downgraded,omitempty"`

	// prompt_layers
	//
	// Layers names what the run was told before the conversation started, and how much of
	// each. Like sandbox_boundary, the record is written even when there is nothing beyond
	// the runtime prompt: an absent record would read as "nobody looked" rather than "there
	// was nothing else".
	Layers []agent.PromptLayer `json:"layers,omitempty"`

	// plugins
	//
	// Plugins names what a run loaded from outside the workspace and the user's
	// own configuration, and for a repository plugin whether that input could
	// still change under it. The record is written even when nothing was
	// loaded, for the same reason as the two above: absence would read as
	// "nobody looked".
	Plugins []plugin.Provenance `json:"plugins,omitempty"`

	// iter_start, llm_start, llm_end
	Iter int `json:"iter,omitempty"`

	// llm_start, llm_end
	ContextTokens int `json:"context_tokens,omitempty"`
	ContextWindow int `json:"context_window,omitempty"`

	// llm_end, run_end
	HasToolCalls     bool `json:"has_tool_calls,omitempty"`
	PromptTokens     int  `json:"prompt_tokens,omitempty"`
	CompletionTokens int  `json:"completion_tokens,omitempty"`
	// CacheReadTokens and CacheWriteTokens are the provider-reported cached
	// parts of the prompt so far. They break PromptTokens down rather than add
	// to it. Absent means the provider reported none, which a reader must not
	// read as a cache miss: a provider that reports nothing is not a provider
	// that missed.
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`

	// llm_end
	Content string `json:"content,omitempty"`

	// tool_start, tool_end, tool_denied
	Tool       string `json:"tool,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Args       string `json:"args,omitempty"`

	// tool_end
	Result     string `json:"result,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`

	// tool_denied
	DenyReason string `json:"deny_reason,omitempty"`

	// context_compacted
	Summarized int `json:"summarized,omitempty"`
	Kept       int `json:"kept,omitempty"`

	// run_end
	ToolCalls int    `json:"tool_calls,omitempty"`
	Error     string `json:"error,omitempty"`
}

// recordTypes maps each agent.EventKind to its trace record type string.
// EventLLMDelta has no entry: streaming deltas are not persisted (redundant
// with llm_end content).
var recordTypes = map[agent.EventKind]string{
	agent.EventIterStart:        "iter_start",
	agent.EventLLMStart:         "llm_start",
	agent.EventLLMEnd:           "llm_end",
	agent.EventToolStart:        "tool_start",
	agent.EventToolEnd:          "tool_end",
	agent.EventToolDenied:       "tool_denied",
	agent.EventContextCompacted: "context_compacted",
	agent.EventRunEnd:           "run_end",
	agent.EventUserInput:        "user_input",
	agent.EventUserInputBlocked: "user_input_blocked",
}

// recordFromEvent maps a runtime event to a trace Record, applying bounding and
// redaction to free-text fields. It returns ok=false for events that are not
// persisted (e.g. EventLLMDelta).
func recordFromEvent(e agent.Event, maxField int) (Record, bool) {
	typ, ok := recordTypes[e.Kind]
	if !ok {
		return Record{}, false
	}
	r := Record{TS: now(), Type: typ}
	switch e.Kind {
	case agent.EventIterStart:
		r.Iter = e.Iter
	case agent.EventLLMStart:
		r.Iter = e.Iter
		r.ContextTokens = e.ContextTokens
		r.ContextWindow = e.ContextWindow
	case agent.EventLLMEnd:
		r.Iter = e.Iter
		r.HasToolCalls = e.HasToolCalls
		r.PromptTokens = e.PromptTokens
		r.CompletionTokens = e.CompletionTokens
		r.CacheReadTokens = e.CacheReadTokens
		r.CacheWriteTokens = e.CacheWriteTokens
		r.ContextTokens = e.ContextTokens
		r.ContextWindow = e.ContextWindow
		r.Content = bound(Redact(e.Content), maxField)
	case agent.EventToolStart:
		r.Tool = e.ToolName
		r.ToolCallID = e.ToolCallID
		r.Args = bound(Redact(e.ToolArgs), maxField)
	case agent.EventToolEnd:
		r.Tool = e.ToolName
		r.ToolCallID = e.ToolCallID
		r.Args = bound(Redact(e.ToolArgs), maxField)
		r.Result = bound(Redact(e.ToolResult), maxField)
		r.DurationMS = e.ToolDuration.Milliseconds()
	case agent.EventToolDenied:
		r.Tool = e.ToolName
		r.ToolCallID = e.ToolCallID
		r.DenyReason = e.DenyReason
	case agent.EventContextCompacted:
		r.Summarized = e.Summarized
		r.Kept = e.Kept
	case agent.EventUserInput:
		// A message that entered the run after it started is part of what the run
		// was told to do, so a trace that omitted it would misreport its instructions.
		r.Iter = e.Iter
		r.Content = bound(Redact(e.Content), maxField)
	case agent.EventUserInputBlocked:
		r.Iter = e.Iter
		r.Content = bound(Redact(e.Content), maxField)
		r.DenyReason = e.DenyReason
	case agent.EventRunEnd:
		r.ToolCalls = e.Stats.ToolCalls
		r.PromptTokens = e.Stats.PromptTokens
		r.CompletionTokens = e.Stats.CompletionTokens
		r.CacheReadTokens = e.Stats.CacheReadTokens
		r.CacheWriteTokens = e.Stats.CacheWriteTokens
		if e.Err != nil {
			r.Error = e.Err.Error()
		}
	}
	return r, true
}

// layersRecord reports the system-prompt layers this run loaded. Written for every run,
// including one that loaded nothing beyond the runtime prompt: trust-harness §3.6 asks that a
// run be able to say which instruction sources it received, and silence is not an answer.
func layersRecord(layers []agent.PromptLayer) Record {
	return Record{TS: now(), Type: "prompt_layers", Layers: append([]agent.PromptLayer(nil), layers...)}
}

// pluginsRecord builds the plugins record written immediately after run_start.
func pluginsRecord(plugins []plugin.Provenance) Record {
	return Record{TS: now(), Type: "plugins", Plugins: append([]plugin.Provenance(nil), plugins...)}
}

// boundaryRecord builds the sandbox_boundary record written immediately after
// run_start.
//
// It is written for every run, including one that was not confined at all.
// Portal run diagnostics and `buildmax sandbox overrides` both answer "what
// contained this run" from this record, and an absent record would be read as
// "nobody checked" rather than "nothing contained it".
func boundaryRecord(info *agent.SandboxInfo) Record {
	rec := Record{TS: now(), Type: "sandbox_boundary", Backend: "none"}
	sandboxed := false
	// A nil view means the surface wired no sandbox, which gives the run no
	// boundary. Recording that as unknown would let an unprotected run read as
	// an unexamined one, so it is reported as unsandboxed.
	if info != nil {
		sandboxed = info.Enabled
		rec.SandboxMode = info.Mode
		if info.Backend != "" {
			rec.Backend = info.Backend
		}
		rec.Sources = append([]string(nil), info.Sources...)
		rec.Downgraded = info.Downgraded
	}
	rec.Sandboxed = &sandboxed
	return rec
}

// bound truncates s to at most max bytes, appending a marker noting how many
// bytes were dropped. max <= 0 disables bounding. The cut backs off to a rune
// boundary so a multi-byte character is never split in half — a partial rune
// would be re-encoded as U+FFFD by the JSON encoder, mangling the last
// character of any non-ASCII field.
func bound(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	dropped := len(s) - cut
	return s[:cut] + fmt.Sprintf(" … [truncated %d bytes]", dropped)
}

// now is overridable in tests.
var now = func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
