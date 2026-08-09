package agent

import (
	"crypto/md5"
	"encoding/json"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ToolPolicy is a configured override layer consulted before tool-declared checks.
// Returning ToolActionAllow defers the decision to the tool's own ArgChecker / PolicyProvider.
// Returning ToolActionDeny or ToolActionAsk overrides the tool entirely.
type ToolPolicy interface {
	Check(name string, args map[string]any) llm.ToolAction
}

// ApprovalHandler is invoked when the resolved action is ToolActionAsk.
// Returns true to allow execution, false to deny.
// If nil, Ask collapses to Deny.
type ApprovalHandler interface {
	RequestApproval(name string, args map[string]any) bool
}

// AllowAllPolicy defers all decisions to each tool's own declarations.
var AllowAllPolicy ToolPolicy = allowAll{}

type allowAll struct{}

func (allowAll) Check(_ string, _ map[string]any) llm.ToolAction { return llm.ToolActionAllow }

// defaultMaxRepeatedCalls is the loop guard threshold: the same tool+args combination
// is blocked after this many repetitions within one run.
const defaultMaxRepeatedCalls = 3

// loopGuard tracks per-fingerprint call counts to detect doom loops.
type loopGuard struct {
	counts map[string]int
	max    int
}

func newLoopGuard(max int) *loopGuard {
	return &loopGuard{counts: make(map[string]int), max: max}
}

// exceeded returns true when the tool+args fingerprint has been seen more than max times.
func (g *loopGuard) exceeded(name string, args map[string]any) bool {
	fp := toolFingerprint(name, args)
	g.counts[fp]++
	return g.counts[fp] > g.max
}

func toolFingerprint(name string, args map[string]any) string {
	b, _ := json.Marshal(args)
	sum := md5.Sum(append([]byte(name+":"), b...))
	return fmt.Sprintf("%x", sum)
}
