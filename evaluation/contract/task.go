package contract

import "encoding/json"

// Domain is the evaluation question a task is built to answer. The four
// domains keep separate scorecards: section 7.5 forbids the weighted average
// that would let a capability gain pay for a trust violation.
type Domain string

const (
	DomainCapability     Domain = "capability"
	DomainReliability    Domain = "reliability"
	DomainTrust          Domain = "trust"
	DomainProductOutcome Domain = "product_outcome"
)

// Surface is the execution adapter a task runs through. It belongs to the task
// rather than to the experiment because a task written for worker artifacts
// does not become a local task by being run locally; cross-surface parity
// compares two tasks stating the same abstract goal, not one task run twice.
type Surface string

const (
	SurfaceAgentCore    Surface = "agent_core"
	SurfaceCLI          Surface = "cli"
	SurfaceDesktop      Surface = "desktop"
	SurfaceWorker       Surface = "worker"
	SurfaceConversation Surface = "conversation"
	SurfaceDeployment   Surface = "deployment"
	SurfaceHarbor       Surface = "harbor"
)

// Task directory layout. The split is the hidden-grader boundary from section
// 18.4, expressed as a convention rather than a per-task path list: a task
// cannot leak its grader by misconfiguring a field, because only StateDir is
// ever materialized into the trial workspace.
const (
	// TaskFile is the task definition, read by the runner and never copied.
	TaskFile = "task.json"
	// StateDir holds the visible initial state. Its contents, and nothing
	// else, become the trial workspace.
	StateDir = "state"
	// GradersDir holds grader material the Agent must not read.
	GradersDir = "graders"
	// OracleDir holds the executable reference solution.
	OracleDir = "oracle"
)

// Materialized reports whether a task-directory entry belongs in the trial
// workspace. Everything outside StateDir stays behind the trial boundary,
// including files a task author adds later without updating this package.
func Materialized(name string) bool { return name == StateDir }

// Task is one evaluation case: what is asked, what state it starts from, what
// bounds it, and what must be true afterwards.
type Task struct {
	ContractVersion int      `json:"contract_version"`
	ID              string   `json:"id"`
	Version         int      `json:"version"`
	Suite           string   `json:"suite"`
	Title           string   `json:"title"`
	Tags            []string `json:"tags,omitempty"`
	Domain          Domain   `json:"domain"`
	Surface         Surface  `json:"surface"`

	// Turns is what the user says, in order. A single-turn task has one entry.
	// Modelling every task as a sequence avoids a second code path for the
	// multi-turn scenarios section 11 requires; a richer turn, such as a
	// simulated user with a policy, extends this field rather than adding one.
	Turns []string `json:"turns"`

	// Capabilities are what the subject must provide for the task to mean
	// anything, such as a sandbox backend or a configured MCP server. A subject
	// missing one yields invalid_task, not a failed attempt: the task was never
	// asked under the conditions it describes.
	Capabilities []string `json:"capabilities,omitempty"`

	Limits Limits `json:"limits"`

	// Graders run in the order given. A required grader that fails decides the
	// trial; an optional one contributes a dimension without gating.
	Graders []GraderRef `json:"graders"`

	// Negative marks a task whose required outcome is that something did not
	// happen: a boundary held, a file was left alone, a tool was never reached.
	//
	// Such a task's deterministic graders legitimately pass against the
	// untouched initial state, because changing nothing is the correct answer,
	// so preflight's "does not already satisfy the outcome" check does not
	// apply to it. What must apply instead is a required trace or model grader:
	// without one the task asserts only that nothing happened, which an agent
	// that did nothing at all — or never ran — would satisfy just as well.
	Negative bool `json:"negative,omitempty"`

	// Trials is the default independent-attempt count. An experiment may raise
	// it; a task only meaningful over repetition — anything measuring pass^k —
	// says so here rather than relying on the caller to know.
	Trials int `json:"trials"`

	// Environment pins the versions a trial depends on beyond the subject, such
	// as a container image or a language toolchain. It is copied onto the
	// bundle so a later reader can tell an environment change from a subject
	// change.
	Environment map[string]string `json:"environment,omitempty"`

	// Oracle is the command completing the task from the initial state, run
	// from OracleDir. Preflight requires it to pass every required grader: a
	// task whose own reference solution fails is measuring its graders rather
	// than the Agent.
	Oracle []string `json:"oracle,omitempty"`
}

// Limits bound one trial. Each is a stop condition rather than a target, and
// exhausting one produces StatusTimedOut rather than a grader failure, so a
// task merely too small to finish in is not reported as an Agent that could
// not finish it.
type Limits struct {
	WallSeconds int `json:"wall_seconds"`
	Iterations  int `json:"iterations,omitempty"`
	ToolCalls   int `json:"tool_calls,omitempty"`
	Tokens      int `json:"tokens,omitempty"`
}

// GraderKind separates the three sources of judgement section 10 keeps apart.
// It is recorded on every result because section 10.3 forbids a model grader
// from overruling a deterministic failure, and a reader cannot enforce that
// without knowing which produced which.
type GraderKind string

const (
	// GraderDeterministic checks final state, artifacts, or contracts.
	GraderDeterministic GraderKind = "deterministic"
	// GraderTrace asserts over recorded process events.
	GraderTrace GraderKind = "trace"
	// GraderModel scores a dimension no state check reduces.
	GraderModel GraderKind = "model"
)

// GraderRef names one grader a task applies, and the configuration it is
// applied under.
type GraderRef struct {
	Name    string     `json:"name"`
	Version int        `json:"version"`
	Kind    GraderKind `json:"kind"`
	// Required means the trial fails when this grader fails. An optional
	// grader reports a dimension without deciding the outcome.
	Required bool `json:"required"`
	// Critical marks a hard gate. Section 7.5 keeps these out of any suite
	// summary, so a critical failure cannot be averaged away.
	Critical bool `json:"critical,omitempty"`
	// Config is the grader's own parameters, opaque here. Graders version
	// separately from the contract, so their schemas must not force a contract
	// version bump.
	Config json.RawMessage `json:"config,omitempty"`
}
