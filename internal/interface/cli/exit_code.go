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
