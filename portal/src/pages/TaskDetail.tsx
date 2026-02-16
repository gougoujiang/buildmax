import Markdown from "react-markdown"
import type { Task } from "../lib/types"

interface TaskDetailProps {
  task: Task
}

export function TaskDetail({ task }: TaskDetailProps) {
  return (
    <div className="page-task">
      <header className="page-task__header">
        <h1 className="page-task__title">Task: {task.title}</h1>
        <button
          type="button"
          className="page-task__restore-btn"
          disabled
          title="Restore is not yet available"
        >
          Restore
        </button>
      </header>

      {/* Result — rendered as markdown */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Result</h2>
        <div className="page-task__markdown">
          <Markdown>{task.summary}</Markdown>
        </div>
      </section>

      {/* What changed — not yet available from backend */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">What changed</h2>
        <p className="page-task__text page-task__muted">Not yet available.</p>
      </section>

      {/* Evidence / Data used — not yet available from backend */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Evidence / Data used</h2>
        <p className="page-task__text page-task__muted">Not yet available.</p>
      </section>

      {/* Meta */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Details</h2>
        <p className="page-task__meta">
          Status: <strong>{task.status}</strong> &middot; {task.timeLabel}
        </p>
      </section>
    </div>
  )
}
