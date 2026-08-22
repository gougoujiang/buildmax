package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/proc"
)

// defaultJobOutputBytes bounds one JobOutput read. Callers page with cursor;
// the job's ring already bounds total retention.
const defaultJobOutputBytes = 20_000

// JobList lists this runtime's background jobs.
type JobList struct{ jobs *job.Manager }

func NewJobList(m *job.Manager) *JobList { return &JobList{jobs: m} }

func (j *JobList) Name() string { return ToolNameJobList }

func (j *JobList) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (j *JobList) Description() string {
	return "List background jobs in this workspace: ID, state, age, and command. Use JobOutput to inspect one."
}

func (j *JobList) Parameters() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (j *JobList) Execute(_ context.Context, _ map[string]any) (string, error) {
	jobs := j.jobs.List()
	if len(jobs) == 0 {
		return "No background jobs.", nil
	}
	var b strings.Builder
	for _, item := range jobs {
		fmt.Fprintf(&b, "%s  %-9s  %-8s  %s\n", item.ID, string(item.State)+stateDetail(item), age(item), summarizeCommand(item.Command))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// JobOutput reports one job's status and incremental output.
type JobOutput struct{ jobs *job.Manager }

func NewJobOutput(m *job.Manager) *JobOutput { return &JobOutput{jobs: m} }

func (j *JobOutput) Name() string { return ToolNameJobOutput }

func (j *JobOutput) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (j *JobOutput) Description() string {
	return "Read a background job's status and captured output incrementally. Pass the cursor from the previous read to continue; only a bounded recent window is retained."
}

func (j *JobOutput) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "The job ID returned when the job started (jb_...)",
			},
			"stream": map[string]any{
				"type":        "string",
				"enum":        []string{"stdout", "stderr"},
				"description": "Which output stream to read (default stdout)",
			},
			"cursor": map[string]any{
				"type":        "number",
				"description": "Continue reading from this offset; use next_cursor from the previous read. 0 (default) reads the oldest retained output.",
			},
		},
		"required": []string{"job_id"},
	}
}

func (j *JobOutput) Execute(_ context.Context, args map[string]any) (string, error) {
	id, err := parseRequiredString(args, "job_id")
	if err != nil {
		return "", err
	}
	item, ok := j.jobs.Get(id)
	if !ok {
		return "No such job: " + id + ". Use JobList to see current jobs.", nil
	}
	stream := proc.Stdout
	if s, _ := args["stream"].(string); s == "stderr" {
		stream = proc.Stderr
	}
	var cursor uint64
	if v, ok := args["cursor"]; ok && v != nil {
		if f, ok := toFloat64(v); ok && f > 0 {
			cursor = uint64(f)
		}
	}
	chunk, err := j.jobs.Output(id, stream, cursor, defaultJobOutputBytes)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "job: %s\nstate: %s%s\ncommand: %s\n", item.ID, item.State, stateDetail(item), summarizeCommand(item.Command))
	if item.Running() {
		fmt.Fprintf(&b, "running for: %s\n", age(item))
	}
	if chunk.Dropped > 0 {
		fmt.Fprintf(&b, "note: %d bytes before this point are no longer retained\n", chunk.Dropped)
	}
	fmt.Fprintf(&b, "--- %s ---\n", streamName(stream))
	b.Write(chunk.Data)
	if len(chunk.Data) == 0 {
		b.WriteString("(no new output)\n")
	} else if !strings.HasSuffix(string(chunk.Data), "\n") {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "--- next_cursor: %d ---", chunk.Next)
	return b.String(), nil
}

// JobStop stops one background job.
type JobStop struct{ jobs *job.Manager }

func NewJobStop(m *job.Manager) *JobStop { return &JobStop{jobs: m} }

func (j *JobStop) Name() string { return ToolNameJobStop }

// Access declares write: stopping changes state. DefaultAction keeps it
// prompt-free — it only ever terminates jobs this runtime itself started,
// which the user can equally do from the activity view.
func (j *JobStop) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

func (j *JobStop) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

func (j *JobStop) Description() string {
	return "Stop a background job. Termination is polite first and escalates to a kill of the job's whole process tree after a grace period."
}

func (j *JobStop) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "The job ID to stop (jb_...)",
			},
		},
		"required": []string{"job_id"},
	}
}

func (j *JobStop) Execute(_ context.Context, args map[string]any) (string, error) {
	id, err := parseRequiredString(args, "job_id")
	if err != nil {
		return "", err
	}
	item, ok := j.jobs.Get(id)
	if !ok {
		return "No such job: " + id + ". Use JobList to see current jobs.", nil
	}
	if !item.Running() {
		return fmt.Sprintf("Job %s already finished (state: %s%s).", id, item.State, stateDetail(item)), nil
	}
	if err := j.jobs.Stop(id); err != nil {
		return "Cannot stop job: " + err.Error(), nil
	}
	return "Stop requested for " + id + ". It escalates to a kill after a short grace period; check the final state with JobOutput.", nil
}

func stateDetail(j job.Job) string {
	switch {
	case j.State == job.StateCanceled && j.StopReason != "":
		return " (" + string(j.StopReason) + ")"
	case j.State == job.StateFailed && j.Err != "":
		return " (" + j.Err + ")"
	}
	return ""
}

func age(j job.Job) string {
	end := j.EndedAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(j.CreatedAt).Round(time.Second).String()
}

func summarizeCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\n", " ")
	if len(cmd) > 80 {
		return cmd[:77] + "..."
	}
	return cmd
}

func streamName(s proc.Stream) string {
	if s == proc.Stderr {
		return "stderr"
	}
	return "stdout"
}
