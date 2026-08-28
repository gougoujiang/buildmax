package contract

import "time"

// TrialStatus is how one attempt ended. Section 7.4 is the whole point of the
// enumeration: Agent failure, invalid task, infrastructure failure, grader
// failure, timeout, and cancellation are different facts, and collapsing them
// into pass/fail reports provider outages and broken tasks as incapability.
type TrialStatus string

const (
	// StatusPassed means every required grader passed.
	StatusPassed TrialStatus = "passed"
	// StatusFailed means execution completed and a required grader failed.
	StatusFailed TrialStatus = "failed"
	// StatusAgentError means the Agent runtime failed before producing a
	// gradable outcome.
	StatusAgentError TrialStatus = "agent_error"
	// StatusInfrastructureError means the environment or a dependency failed
	// independently of Agent capability.
	StatusInfrastructureError TrialStatus = "infrastructure_error"
	// StatusGraderError means required grading could not complete, so the
	// attempt is unscored rather than failed.
	StatusGraderError TrialStatus = "grader_error"
	// StatusTimedOut means a stated budget expired: the task's wall time, or
	// the iteration cap it asked the subject to run under. Both are the task
	// saying how much the answer may cost, so a subject that ran out of either
	// failed the task as written rather than hitting a broken harness.
	StatusTimedOut TrialStatus = "timed_out"
	// StatusCanceled means the experiment controller stopped the trial.
	StatusCanceled TrialStatus = "canceled"
	// StatusInvalidTask means preflight or task integrity failed, so nothing
	// about the subject was measured.
	StatusInvalidTask TrialStatus = "invalid_task"
)

// Scored reports whether the status is a judgement about the subject. Only
// passed, failed, and timed_out are: a trial the harness could not run says
// nothing about capability, and a rate computed over the rest understates a
// subject in proportion to how unreliable the harness was that day.
//
// Timeouts count because a budget is part of the task. A subject that cannot
// finish inside the stated limit has failed the task as written.
func (s TrialStatus) Scored() bool {
	switch s {
	case StatusPassed, StatusFailed, StatusTimedOut:
		return true
	default:
		return false
	}
}

// HarnessFault reports whether the status blames the harness rather than the
// subject. Section 12 requires these reported as their own rates instead of
// disappearing, because a suite quietly dropping a third of its trials looks
// identical to one that ran clean.
func (s TrialStatus) HarnessFault() bool {
	switch s {
	case StatusInfrastructureError, StatusGraderError, StatusInvalidTask:
		return true
	default:
		return false
	}
}

// RetentionLevel is how much of a trial's free text the bundle keeps. Section
// 7.6 keeps bundles local by default; retention is what makes an export
// bounded rather than a copy of a private workspace.
type RetentionLevel string

const (
	// RetentionFull keeps replies, tool arguments, and results as recorded.
	RetentionFull RetentionLevel = "full"
	// RetentionBounded keeps them truncated and redacted.
	RetentionBounded RetentionLevel = "bounded"
	// RetentionMetadata keeps no free text at all: statuses, digests, counts,
	// and grader verdicts only. This is the level an export defaults to.
	RetentionMetadata RetentionLevel = "metadata"
)

// TrialBundle is the canonical interchange record for one attempt, and the
// stable boundary between runners, graders, and viewers. Qualification gates
// read only the fields defined here; an extension may add its own without
// becoming something a gate depends on.
type TrialBundle struct {
	ContractVersion int    `json:"contract_version"`
	TrialID         string `json:"trial_id"`
	ExperimentID    string `json:"experiment_id"`
	TaskID          string `json:"task_id"`
	TaskVersion     int    `json:"task_version"`
	Suite           string `json:"suite"`
	SubjectID       string `json:"subject_id"`
	// Index is which independent attempt this is, from zero. Paired comparison
	// matches candidate and baseline on task and index, so it has to be
	// recorded rather than inferred from file order.
	Index int `json:"index"`

	Domain  Domain  `json:"domain"`
	Surface Surface `json:"surface"`

	Status TrialStatus `json:"status"`
	// FailureClass refines Status for a failure, such as a boundary violation
	// or an early stop. It is free-form because the taxonomy is expected to
	// grow from observed failures rather than be enumerated in advance; a gate
	// reads Status, and a human reads this.
	FailureClass string `json:"failure_class,omitempty"`
	// Error is what went wrong for a status that is not a grader verdict.
	Error string `json:"error,omitempty"`

	StartedAt time.Time `json:"started_at"`
	Duration  Duration  `json:"duration"`

	// InitialStateDigest is the workspace the trial started from. Without it a
	// re-run cannot prove it began where the recorded one did, and section 8.1
	// requires an initial state that does not already satisfy the outcome.
	InitialStateDigest string `json:"initial_state_digest"`
	// FinalStateDigest is what the workspace became. It is the outcome-first
	// evidence of section 7.1: a reply claiming a file was written is not the
	// file.
	FinalStateDigest string `json:"final_state_digest,omitempty"`

	Retention RetentionLevel `json:"retention"`
	// Reply is the subject's final answer, subject to Retention. Absent at
	// metadata retention, which is not the same fact as a run that said
	// nothing.
	Reply string `json:"reply,omitempty"`

	// TracePath is the durable trace inside the bundle directory, relative to
	// it. Children are the subagent traces the run spawned, so a delegation
	// failure is diagnosable from the bundle alone.
	TracePath      string   `json:"trace_path,omitempty"`
	ChildTracePath []string `json:"child_trace_paths,omitempty"`

	Artifacts []ArtifactRef  `json:"artifacts,omitempty"`
	Graders   []GraderResult `json:"graders,omitempty"`
	Usage     Usage          `json:"usage"`

	// Reproduce is the bounded description of how to run this trial again:
	// the command, the environment it needed, and the dataset it came from.
	// Section 17 makes it part of what a failure has to hand a contributor.
	Reproduce Reproduction `json:"reproduce"`
}

// ArtifactRef identifies something the trial produced, by hash rather than by
// content, so a bundle stays bounded while still proving what was made.
type ArtifactRef struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
	// Verified is the grader output that checked it, empty when nothing did.
	Verified string `json:"verified,omitempty"`
}

// Usage is what the trial consumed. Cost is deliberately absent unless priced:
// section 12 admits cost only when pricing input is explicit and versioned,
// and a zero would read as free rather than as unpriced.
type Usage struct {
	LLMCalls         int `json:"llm_calls"`
	ToolCalls        int `json:"tool_calls"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	// Cost is nano-units of Currency, matching the runtime's integer
	// representation so a reader sums a suite exactly. Absent when the model
	// was unpriced.
	Cost     *int64 `json:"cost,omitempty"`
	Currency string `json:"currency,omitempty"`
	// CostIncomplete says part of the trial could not be priced, so Cost
	// understates it rather than covering it.
	CostIncomplete bool `json:"cost_incomplete,omitempty"`
}

// Reproduction is the bounded path back to this trial.
type Reproduction struct {
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment,omitempty"`
	Dataset     DatasetRef        `json:"dataset"`
	// Note is anything a re-runner needs that the command does not carry, such
	// as a dependency that must already be installed.
	Note string `json:"note,omitempty"`
}

// Duration is a wall-clock span serialised as milliseconds. time.Duration
// marshals as nanoseconds, which is precision this format cannot honour and a
// reader outside Go has to know to divide; milliseconds is what the runtime's
// own JSON already reports.
type Duration int64

// FromDuration converts a Go duration for storage.
func FromDuration(d time.Duration) Duration { return Duration(d.Milliseconds()) }

// Duration converts back.
func (d Duration) Duration() time.Duration { return time.Duration(d) * time.Millisecond }
