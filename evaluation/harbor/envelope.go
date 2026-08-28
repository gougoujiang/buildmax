package harbor

import (
	"os"
	"path/filepath"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// agentEnvelope is BuildMax's own print-mode result, as the adapter left it in
// the trial's agent directory.
//
// It is redeclared rather than shared with evaluation/adapter for the reason
// that package gives for redeclaring it there: the CLI's type is unexported,
// and the envelope is a documented output contract rather than an internal
// shape. What holds the three copies together is the end-to-end tests that run
// a real binary.
type agentEnvelope struct {
	SessionID string `json:"session_id"`
	TraceID   string `json:"trace_id"`
	TracePath string `json:"trace_path"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Reply     string `json:"reply"`
	ToolCalls int    `json:"tool_calls"`
	Usage     struct {
		TotalPrompt     int `json:"total_prompt"`
		TotalCompletion int `json:"total_completion"`
		TotalCacheRead  int `json:"total_cache_read"`
		TotalCacheWrite int `json:"total_cache_write"`
		Cost            *struct {
			Currency string `json:"currency"`
			Total    int64  `json:"total"`
		} `json:"cost"`
		CostIncomplete bool `json:"cost_incomplete"`
	} `json:"usage"`
	ExitCode int `json:"exit_code"`
	Error    *struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	} `json:"error"`
}

// exitIterationCap mirrors cli.ExitIterationCap. It is repeated rather than
// imported because evaluation reads the binary's documented contract with a
// shell, not the package that implements it.
const exitIterationCap = 7

func (e agentEnvelope) stoppedAtIterationCap() bool { return e.ExitCode == exitIterationCap }

func (e agentEnvelope) usage() contract.Usage {
	u := contract.Usage{
		ToolCalls:        e.ToolCalls,
		PromptTokens:     e.Usage.TotalPrompt,
		CompletionTokens: e.Usage.TotalCompletion,
		CacheReadTokens:  e.Usage.TotalCacheRead,
		CacheWriteTokens: e.Usage.TotalCacheWrite,
		CostIncomplete:   e.Usage.CostIncomplete,
	}
	if e.Usage.Cost != nil {
		total := e.Usage.Cost.Total
		u.Cost = &total
		u.Currency = e.Usage.Cost.Currency
	}
	return u
}

// readEnvelope loads BuildMax's own report of a trial. It returns nil, and a
// reason, when there is nothing to read.
//
// Nothing here fails the import. An attempt that produced no usable envelope is
// still an attempt Harbor recorded and judged, and refusing it would throw away
// every other trial in the job over one file.
//
// The shapes it has to survive were found by running the benchmark. A container
// that never started leaves no file. A subject killed mid-run leaves an empty
// one, because the shell creates it with `>` before the binary writes a byte —
// so empty means "the run did not get that far", not "the contract is broken".
// A truncated file is the same event caught a moment later.
func readEnvelope(t Trial) (*agentEnvelope, string) {
	path := filepath.Join(t.AgentDir(), AgentResultFile)
	info, err := os.Stat(path)
	if err != nil {
		return nil, ""
	}
	if info.Size() == 0 {
		return nil, "the subject wrote no result envelope, so it did not finish"
	}
	var env agentEnvelope
	if err := readForeignJSON(path, &env); err != nil {
		return nil, "the subject's result envelope did not parse: " + err.Error()
	}
	return &env, ""
}
