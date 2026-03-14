package entity

// RunStatus is the canonical lifecycle status for chat runs.
type RunStatus string

const (
	RunStatusPending   RunStatus = "PENDING"
	RunStatusScheduled RunStatus = "SCHEDULED"
	RunStatusRunning   RunStatus = "RUNNING"
	RunStatusSucceeded RunStatus = "SUCCEEDED"
	RunStatusFailed    RunStatus = "FAILED"
)

// ClaimChatRunInput atomically transitions a run from ExpectedStatus to NewStatus.
type ClaimChatRunInput struct {
	ChatRunID      string
	ExpectedStatus RunStatus
	NewStatus      RunStatus
	StartedAt      *int64
	EndedAt        *int64
	Output         *string
	ErrorMessage   *string
	SessionID      *string
}

// UpdateChatRunInput updates a run to the given status with optional fields.
type UpdateChatRunInput struct {
	ChatRunID        string
	Status           RunStatus
	StartedAt        *int64
	EndedAt          *int64
	Output           *string
	ErrorMessage     *string
	SessionID        *string
	PromptTokens     *int
	CompletionTokens *int
}
