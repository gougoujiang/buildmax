import type { Artifact, Task, ViewArtifactParams } from "../lib/types"
import { taskStatusIcon } from "../lib/taskStatus"
import { navigate } from "../router"

interface ActivityProps {
  workspaceId: string
  tasks: Task[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function Activity({
  workspaceId,
  tasks,
  artifacts,
  onViewArtifact,
}: ActivityProps) {
  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Activity</h1>
      <p className="page-activity__subtitle">
        All tasks and artifacts in this workspace.
      </p>
      {tasks.length === 0 && artifacts.length === 0 ? (
        <p className="page-activity__empty">No activity yet.</p>
      ) : (
        <>
          {tasks.length > 0 && (
            <ul className="page-activity__list">
              {tasks.map((task) => (
                <li key={task.id} className="page-activity__item">
                  <button
                    type="button"
                    className="page-activity__link"
                    onClick={() =>
                      navigate({
                        name: "task",
                        workspaceId,
                        taskId: task.id,
                      })
                    }
                  >
                    <span className="page-activity__icon">
                      {taskStatusIcon(task.status)}
                    </span>
                    <span className="page-activity__content">
                      <span className="page-activity__task-title">{task.title}</span>
                      <span className="page-activity__meta">{task.timeLabel}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {artifacts.length > 0 && (
            <section className="page-activity__artifacts">
              <h2 className="page-activity__artifacts-heading">Artifacts</h2>
              <ul className="page-activity__artifact-list">
                {artifacts.map((a) => (
                  <li key={`artifact-${a.id}`} className="page-activity__artifact-item">
                    <span className="page-activity__content">
                      <span className="page-activity__task-title">{a.title}</span>
                      <span className="page-activity__meta">
                        {a.timeLabel} · task: {a.taskId} · artifact: {a.id}
                      </span>
                    </span>
                    {onViewArtifact && (
                      <button
                        type="button"
                        className="page-activity__artifact-view"
                        onClick={() => onViewArtifact({ workspaceId, artifactId: a.id })}
                      >
                        View
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </div>
  )
}
