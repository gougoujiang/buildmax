import type { Project, Task } from "./types"

export function getProjectById(projects: Project[], projectId: string): Project | undefined {
  return projects.find((p) => p.id === projectId)
}

export function getProjectName(projects: Project[], projectId: string): string {
  return projects.find((p) => p.id === projectId)?.name ?? "Project"
}

/** Resolve task by id from workspace task list (tasks are workspace-scoped only). */
export function getTaskForDetail(
  workspaceTasks: Task[],
  taskId: string
): Task | undefined {
  return workspaceTasks.find((t) => t.id === taskId)
}
