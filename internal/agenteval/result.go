package agenteval

import (
	"encoding/json"
	"fmt"
	"time"
)

// TaskResult records the outcome of one task run.
type TaskResult struct {
	TaskID    string        `json:"task_id"`
	Title     string        `json:"title"`
	Pass      bool          `json:"pass"`
	Duration  time.Duration `json:"duration_ns"`
	Model     string        `json:"model"`
	Timestamp time.Time     `json:"timestamp"`
	// Token counts for the agent run (all LLM calls summed).
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	ToolCalls        int    `json:"tool_calls"`
	Reply            string `json:"reply,omitempty"`
	GraderOutput     string `json:"grader_output,omitempty"`
	AgentError       string `json:"agent_error,omitempty"`
	GraderError      string `json:"grader_error,omitempty"`
}

// MarshalJSONLine serialises the result as a single-line JSON record.
func (r TaskResult) MarshalJSONLine() ([]byte, error) {
	return json.Marshal(r)
}

// Summary returns a human-readable line for terminal output including token stats.
func (r TaskResult) Summary() string {
	icon := "✓"
	if !r.Pass {
		icon = "✗"
	}
	line := fmt.Sprintf("%s %-48s %s  ↑%-6s ↓%-6s = %s tok",
		icon,
		truncate(r.Title, 48),
		formatDuration(r.Duration),
		formatInt(r.PromptTokens),
		formatInt(r.CompletionTokens),
		formatInt(r.TotalTokens),
	)
	if r.ToolCalls > 0 {
		line += fmt.Sprintf("  [%d calls]", r.ToolCalls)
	}
	if !r.Pass {
		if r.AgentError != "" {
			line += "\n      ↳ agent error: " + truncate(r.AgentError, 100)
		} else if r.GraderError != "" {
			line += "\n      ↳ grader: " + truncate(r.GraderError, 100)
		}
	}
	return line
}

// EvalSummary holds aggregate stats across all tasks in a run.
type EvalSummary struct {
	Total            int
	Passed           int
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	TotalToolCalls   int
	TotalDuration    time.Duration
}

// Add folds one TaskResult into the summary.
func (s *EvalSummary) Add(r TaskResult) {
	s.Total++
	if r.Pass {
		s.Passed++
	}
	s.TotalTokens += r.TotalTokens
	s.PromptTokens += r.PromptTokens
	s.CompletionTokens += r.CompletionTokens
	s.TotalToolCalls += r.ToolCalls
	s.TotalDuration += r.Duration
}

// Print writes the aggregate stats block to stdout.
func (s *EvalSummary) Print() {
	passRate := 0.0
	if s.Total > 0 {
		passRate = float64(s.Passed) / float64(s.Total) * 100
	}
	tokPerTask := 0
	if s.Total > 0 {
		tokPerTask = s.TotalTokens / s.Total
	}
	tokPerPass := 0
	if s.Passed > 0 {
		tokPerPass = s.TotalTokens / s.Passed
	}
	callsPerTask := 0.0
	if s.Total > 0 {
		callsPerTask = float64(s.TotalToolCalls) / float64(s.Total)
	}

	fmt.Println()
	fmt.Printf("Score        : %d/%d (%.0f%%)\n", s.Passed, s.Total, passRate)
	fmt.Printf("Duration     : %s total\n", formatDuration(s.TotalDuration))
	fmt.Printf("Tokens       : %s total  (↑%s prompt  ↓%s completion)\n",
		formatInt(s.TotalTokens),
		formatInt(s.PromptTokens),
		formatInt(s.CompletionTokens),
	)
	fmt.Printf("Efficiency   : %s tok/task  |  %s tok/pass\n",
		formatInt(tokPerTask),
		formatIntOrDash(tokPerPass, s.Passed),
	)
	fmt.Printf("Tool calls   : %d total  (%.1f avg/task)\n", s.TotalToolCalls, callsPerTask)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	// Insert comma separators for readability.
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func formatIntOrDash(n, count int) string {
	if count == 0 {
		return "—"
	}
	return formatInt(n)
}
