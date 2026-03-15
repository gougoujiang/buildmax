import type { Task } from "./types"

/** Resolve task by id from the currently loaded task list. */
export function getTaskForDetail(
  tasks: Task[],
  taskId: string
): Task | undefined {
  return tasks.find((t) => t.id === taskId)
}
