package desktop

import (
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/infra/proc"
)

// eventJobUpdate carries one background-job lifecycle transition to the
// frontend (payload JobPayload).
const eventJobUpdate = "desktop/job-update"

// JobPayload is one background job as the frontend sees it.
type JobPayload struct {
	ProjectID  string `json:"project_id"`
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	StopReason string `json:"stop_reason,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`
	Command    string `json:"command"`
	SessionID  string `json:"session_id,omitempty"`
	Running    bool   `json:"running"`
	CreatedAt  string `json:"created_at"`
	EndedAt    string `json:"ended_at,omitempty"`
}

// JobOutputPayload is one incremental output read.
type JobOutputPayload struct {
	Data       string `json:"data"`
	NextCursor uint64 `json:"next_cursor"`
	Dropped    uint64 `json:"dropped,omitempty"`
	Running    bool   `json:"running"`
	State      string `json:"state"`
}

// desktopJobOutputBytes bounds one bridge read; the frontend pages with the
// returned cursor.
const desktopJobOutputBytes = 64 * 1024

func jobPayload(projectID string, j job.Job) JobPayload {
	p := JobPayload{
		ProjectID:  projectID,
		ID:         j.ID,
		Kind:       string(j.Kind),
		State:      string(j.State),
		StopReason: string(j.StopReason),
		ExitCode:   j.ExitCode,
		Error:      j.Err,
		Command:    j.Command,
		SessionID:  j.Provenance.SessionID,
		Running:    j.Running(),
		CreatedAt:  j.CreatedAt.Format(time.RFC3339),
	}
	if !j.EndedAt.IsZero() {
		p.EndedAt = j.EndedAt.Format(time.RFC3339)
	}
	return p
}

// pumpJobEvents forwards one AgentApp's job lifecycle events to the frontend
// until the manager closes the subscription (when the app closes).
func (a *App) pumpJobEvents(projectID string, m *job.Manager) {
	events, _ := m.Subscribe("")
	go func() {
		for e := range events {
			a.emit(a.ctx, eventJobUpdate, jobPayload(projectID, e.Job))
		}
	}()
}

// jobsForProject resolves the project's job manager, or a clear error on a
// surface state where jobs are unavailable.
func (a *App) jobsForProject(projectID string) (*job.Manager, error) {
	ag, err := a.agentAppForProject(projectID)
	if err != nil {
		return nil, err
	}
	m := ag.Jobs()
	if m == nil {
		return nil, fmt.Errorf("background jobs are not available for this project")
	}
	return m, nil
}

// ListJobs returns the project's background jobs in creation order.
func (a *App) ListJobs(projectID string) ([]JobPayload, error) {
	m, err := a.jobsForProject(projectID)
	if err != nil {
		return nil, err
	}
	jobs := m.List()
	out := make([]JobPayload, len(jobs))
	for i, j := range jobs {
		out[i] = jobPayload(projectID, j)
	}
	return out, nil
}

// StopJob requests termination of one background job.
func (a *App) StopJob(projectID, jobID string) error {
	m, err := a.jobsForProject(projectID)
	if err != nil {
		return err
	}
	return m.Stop(jobID)
}

// GetJobOutput reads one job's captured output incrementally. stream is
// "stdout" (default) or "stderr"; pass the previous next_cursor to continue.
func (a *App) GetJobOutput(projectID, jobID, stream string, cursor uint64) (JobOutputPayload, error) {
	m, err := a.jobsForProject(projectID)
	if err != nil {
		return JobOutputPayload{}, err
	}
	j, ok := m.Get(jobID)
	if !ok {
		return JobOutputPayload{}, fmt.Errorf("no such job: %s", jobID)
	}
	s := proc.Stdout
	if stream == "stderr" {
		s = proc.Stderr
	}
	chunk, err := m.Output(jobID, s, cursor, desktopJobOutputBytes)
	if err != nil {
		return JobOutputPayload{}, err
	}
	return JobOutputPayload{
		Data:       string(chunk.Data),
		NextCursor: chunk.Next,
		Dropped:    chunk.Dropped,
		Running:    j.Running(),
		State:      string(j.State),
	}, nil
}
