import type { Task } from "../lib/types"
import { navigate } from "../lib/router"
import { listTasksForWorkspace, getProjectById } from "../data/mockData"

interface ActivityPageProps {
  workspaceId: string
}

function statusIcon(status: Task["status"]): string {
  switch (status) {
    case "success":
      return "\u2705"
    case "running":
      return "\u23f3"
    case "failed":
      return "\u274c"
    case "canceled":
      return "\u26d4"
  }
}

export function ActivityPage({ workspaceId }: ActivityPageProps) {
  const tasks = listTasksForWorkspace(workspaceId)

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
            const project = getProjectById(task.projectId!)
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
                    {statusIcon(task.status)}
                  </span>
                  <span className="page-activity__content">
                    <span className="page-activity__task-title">{task.title}</span>
                    <span className="page-activity__meta">
                      {project?.name ?? "Project"} · {task.timeLabel}
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
