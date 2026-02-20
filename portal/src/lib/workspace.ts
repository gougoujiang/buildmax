import type { Project, Task } from "./types"

export function getProjectById(projects: Project[], projectId: string): Project | undefined {
  return projects.find((p) => p.id === projectId)
}

export function getProjectName(projects: Project[], projectId: string): string {
  return projects.find((p) => p.id === projectId)?.name ?? "Project"
}

export function getTaskForDetail(
  tasks: Task[],
  workspaceTasks: Task[],
  projectId: string | undefined,
  taskId: string
): Task | undefined {
  const fromProject = projectId ? tasks.find((t) => t.id === taskId) : undefined
  if (fromProject) return fromProject
  return workspaceTasks.find((t) => t.id === taskId)
}
