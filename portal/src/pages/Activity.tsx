import type { Task } from "../lib/types"
import { taskStatusIcon } from "../lib/taskStatus"
import { navigate } from "../lib/router"

interface ActivityProps {
  workspaceId: string
  tasks: Task[]
  getProjectName: (projectId: string) => string
}

export function Activity({ workspaceId, tasks, getProjectName }: ActivityProps) {

  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Activity</h1>
      <p className="page-activity__subtitle">
        All tasks from every project in this workspace.
      </p>
      {tasks.length === 0 ? (
        <p className="page-activity__empty">No activity yet.</p>
      ) : (
        <ul className="page-activity__list">
          {tasks.filter((task) => task.projectId != null).map((task) => {
            const projectName = getProjectName(task.projectId!)
            return (
              <li key={`${task.projectId}-${task.id}`} className="page-activity__item">
                <button
                  type="button"
                  className="page-activity__link"
                  onClick={() =>
                    navigate({
                      name: "task",
                      workspaceId,
                      projectId: task.projectId!,
                      taskId: task.id,
                    })
                  }
                >
                  <span className="page-activity__icon">
                    {taskStatusIcon(task.status)}
                  </span>
                  <span className="page-activity__content">
                    <span className="page-activity__task-title">{task.title}</span>
                    <span className="page-activity__meta">
                      {projectName} · {task.timeLabel}
                    </span>
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
