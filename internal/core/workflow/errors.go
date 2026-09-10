package workflow

import "errors"

// ErrInvalidRunTransition and ErrInvalidStepRunTransition are returned when a
// caller asks a run or step run to move between statuses that
// ValidRunStatusTransition / ValidStepRunTransition do not allow. They name a
// programming error, not a lost race: a refused-but-valid transition (the row
// was no longer at the expected status) is reported as a false result, not an
// error.
var (
	ErrInvalidRunTransition     = errors.New("invalid workflow run status transition")
	ErrInvalidStepRunTransition = errors.New("invalid workflow step run status transition")
)
