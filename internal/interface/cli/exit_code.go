package cli

import "fmt"

// Exit codes for the CLI. Stable contract for shell scripts wrapping
// `buildmax -p`. Documented for users in docs/reference/cli.md.
const (
	ExitOK            = 0
	ExitGeneric       = 1
	ExitUsage         = 2 // bad flag, missing config (e.g. no model configured)
	ExitPolicyDenied  = 3 // tool blocked by configured policy
	ExitModelError    = 4 // LLM/agent runtime error
	ExitToolError     = 5 // reserved
	ExitUserCancelled = 6 // SIGINT / ctx cancelled
	// ExitIterationCap is a run that reached agent.max_iterations. It is
	// separate from ExitModelError because the two ask different things of a
	// caller: a model error is a fault to retry, while an exhausted budget is
	// an answer — the run stopped where it was told to, and whatever it wrote
	// is real. A harness that retried this would pay for the same cap again.
	ExitIterationCap = 7
)

// ExitError wraps an exit code so cobra's RunE can return it and main can
// surface it as the process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitCodeFor returns the exit code embedded in err, or ExitGeneric when err
// is non-nil but not an ExitError, or ExitOK when err is nil.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	if ee, ok := err.(*ExitError); ok {
		return ee.Code
	}
	return ExitGeneric
}
