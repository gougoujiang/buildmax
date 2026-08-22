package agentapp

import (
	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
)

// logJobEvents writes every job event to the durable job log until the
// manager closes the subscription at shutdown. It rides the same
// subscription mechanism as the surfaces, so the manager needs no trace
// knowledge; the subscriber buffer means an extreme burst can skip a record,
// which the trace contract accepts (fail-open) rather than ever slowing a
// job down.
func logJobEvents(dir string, events <-chan job.Event) {
	for e := range events {
		trace.AppendJobRecord(dir, jobTraceRecord(e))
	}
}

func jobTraceRecord(e job.Event) trace.JobRecord {
	j := e.Job
	rec := trace.JobRecord{
		JobID:     j.ID,
		SessionID: j.Provenance.SessionID,
	}
	if e.Type == job.EventMonitorLine {
		rec.Type = "job_line"
		rec.Line = e.Line
		rec.DroppedLines = e.DroppedLines
		return rec
	}
	if j.Running() {
		sandboxed := j.Provenance.Sandboxed
		rec.Type = "job_start"
		rec.Kind = string(j.Kind)
		rec.Command = j.Command
		rec.ParentRunID = j.Provenance.ParentTraceID
		rec.ParentToolCallID = j.Provenance.ParentToolCallID
		rec.Sandboxed = &sandboxed
		rec.Deliver = j.Deliver
		rec.PID = j.PID
		return rec
	}
	rec.Type = "job_end"
	rec.State = string(j.State)
	rec.StopReason = string(j.StopReason)
	rec.ExitCode = j.ExitCode
	rec.Error = j.Err
	return rec
}
