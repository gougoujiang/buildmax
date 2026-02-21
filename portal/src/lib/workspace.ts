import type { Task } from "./types"

/** Resolve task by id from workspace task list (tasks are workspace-scoped only). */
export function getTaskForDetail(
  workspaceTasks: Task[],
  taskId: string
): Task | undefined {
  return workspaceTasks.find((t) => t.id === taskId)
}
