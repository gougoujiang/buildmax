package model

// RunStatus is the canonical lifecycle status for task runs.
type RunStatus string

const (
	RunStatusPending   RunStatus = "PENDING"
	RunStatusScheduled RunStatus = "SCHEDULED"
	RunStatusRunning   RunStatus = "RUNNING"
	RunStatusSucceeded RunStatus = "SUCCEEDED"
	RunStatusFailed    RunStatus = "FAILED"
)

// ClaimTaskRunInput atomically transitions a run from ExpectedStatus to NewStatus.
type ClaimTaskRunInput struct {
	TaskRunID      string
	ExpectedStatus RunStatus
	NewStatus      RunStatus
	StartedAt      *int64
	EndedAt        *int64
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// UpdateTaskInput updates a task to the given status with optional fields.
type UpdateTaskInput struct {
	TaskID       string
	Status       string
	StartedAt    *int64
	EndedAt      *int64
	Output       *string
	ErrorMessage *string
	SessionID    *string
}

// ClaimTaskInput atomically transitions a task from ExpectedStatus to NewStatus.
type ClaimTaskInput struct {
	TaskID         string
	ExpectedStatus string
	NewStatus      string
	StartedAt      *int64
	EndedAt        *int64
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// UpdateTaskRunInput updates a run to the given status with optional fields.
type UpdateTaskRunInput struct {
	TaskRunID        string
	Status           RunStatus
	StartedAt        *int64
	EndedAt          *int64
	Output           *string
	ErrorMessage     *string
	SessionID        *string
	PromptTokens     *int
	CompletionTokens *int
}
