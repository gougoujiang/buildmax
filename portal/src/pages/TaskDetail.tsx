import type { Task } from "../types"

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

      {/* Result */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Result</h2>
        <p className="page-task__text">{task.summary}</p>
      </section>

      {/* What changed */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">What changed</h2>
        <ul className="page-task__change-list">
          <li>Updated relevant datasets</li>
          <li>Generated new outputs based on latest data</li>
          <li>Recorded activity snapshot</li>
        </ul>
      </section>

      {/* Evidence */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Evidence / Data used</h2>
        <ul className="page-task__evidence-list">
          <li>Source datasets from the project</li>
          <li>Previous baseline comparison</li>
        </ul>
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
