import type { ApiTask } from "../../../lib/api/types"
import { taskRunFailed, taskRunFinished } from "../thread"

const previewMaxLen = 600

interface TaskCardProps {
  task: ApiTask
  /** Null while the card's own action is not available (no token yet). */
  busy: boolean
  onStop: (taskId: string) => void
  onRetry: (taskId: string) => void
  onOpenFiles: (taskRunId: string) => void
  onOpenTrace: (taskRunId: string) => void
  onOpenIssue?: (issueId: string) => void
  error?: string | null
}

function statusTone(status: string): "running" | "failed" | "done" {
  if (!taskRunFinished(status)) return "running"
  return taskRunFailed(status) ? "failed" : "done"
}

function statusLabel(status: string): string {
  switch (status.toUpperCase()) {
    case "PENDING":
      return "Queued"
    case "SCHEDULED":
      return "Starting"
    case "RUNNING":
      return "Running"
    case "SUCCEEDED":
    case "SUCCESS":
      return "Done"
    case "FAILED":
      return "Failed"
    case "CANCELED":
      return "Stopped"
    default:
      return status
  }
}

function preview(output: string | null | undefined): string | null {
  if (!output) return null
  const trimmed = output.trim()
  if (trimmed === "") return null
  return trimmed.length > previewMaxLen ? `${trimmed.slice(0, previewMaxLen)}\n…` : trimmed
}

/**
 * One background task, projected into the conversation.
 *
 * It is a card and not a message on purpose: what it shows keeps changing after
 * it first appears, and it was written by a runtime rather than said by anyone.
 * It stands on its own — status, what the run produced, and the way back to the
 * run's files and trace — so a summary that never arrives costs a sentence, not
 * the result.
 */
export function TaskCard({
  task,
  busy,
  onStop,
  onRetry,
  onOpenFiles,
  onOpenTrace,
  onOpenIssue,
  error,
}: TaskCardProps) {
  const tone = statusTone(task.status)
  const finished = taskRunFinished(task.status)
  const body = preview(task.output)
  const artifactRunId = task.artifact_run_ids?.[0]

  return (
    <article className={`task-card task-card--${tone}`}>
      <header className="task-card__head">
        <span className={`task-card__status task-card__status--${tone}`}>{statusLabel(task.status)}</span>
        <span className="task-card__title">{task.title || task.input}</span>
      </header>
      {task.error_message ? <p className="task-card__error">{task.error_message}</p> : null}
      {body ? <pre className="task-card__preview">{body}</pre> : null}
      {!body && !task.error_message && finished ? (
        <p className="task-card__meta">This run finished without output.</p>
      ) : null}
      {error ? <p className="task-card__error">{error}</p> : null}
      <footer className="task-card__actions">
        {!finished ? (
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={busy}
            onClick={() => onStop(task.id)}
          >
            {busy ? "Stopping…" : "Stop"}
          </button>
        ) : (
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={busy}
            onClick={() => onRetry(task.id)}
          >
            {busy ? "Retrying…" : "Run again"}
          </button>
        )}
        {artifactRunId ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenFiles(artifactRunId)}
          >
            Files
          </button>
        ) : null}
        {task.last_run_id ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenTrace(task.last_run_id!)}
          >
            Run details
          </button>
        ) : null}
        {task.issue_id && onOpenIssue ? (
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => onOpenIssue(task.issue_id!)}
          >
            Open issue
          </button>
        ) : null}
      </footer>
    </article>
  )
}
