import type { Artifact, Project, Route, Task, ViewArtifactParams } from "../lib/types"
import { getProjectById, getProjectName, getTaskForDetail } from "../lib/workspace"
import { Activity } from "../pages/Activity"
import { Project as ProjectView } from "../pages/Project"
import { Projects } from "../pages/Projects"
import { TaskDetail } from "../pages/TaskDetail"
import { Explore } from "../pages/Explore"

export type { ViewArtifactParams }

export interface WorkspaceRouterProps {
  route: Route
  projects: Project[]
  tasks: Task[]
  workspaceTasks: Task[]
  artifacts: Artifact[]
  token: string | undefined
  onViewArtifact: (params: ViewArtifactParams) => void
  onRefetchProjects: () => void
  onRefetchTasks: () => void
  onRefetchWorkspaceTasks: () => void
  onRefetchArtifacts: (projectId?: string, taskId?: string) => void
}

export function WorkspaceRouter({
  route,
  projects,
  tasks,
  workspaceTasks,
  artifacts,
  token,
  onViewArtifact,
  onRefetchProjects,
  onRefetchTasks,
  onRefetchWorkspaceTasks,
  onRefetchArtifacts,
}: WorkspaceRouterProps) {
  const projectIdFromRoute = "projectId" in route ? route.projectId : undefined

  const fallbackHome = (
    <Projects
      workspaceId={route.workspaceId}
      projects={projects}
      artifacts={artifacts}
      token={token}
      onRefetchProjects={onRefetchProjects}
      onRefetchWorkspaceTasks={onRefetchWorkspaceTasks}
      onRefetchArtifacts={onRefetchArtifacts}
      onViewArtifact={onViewArtifact}
    />
  )

  const fallbackProject = (project: Project) => (
    <ProjectView
      workspaceId={route.workspaceId}
      project={project}
      tasks={projectIdFromRoute === project.id ? tasks : []}
      artifacts={projectIdFromRoute === project.id ? artifacts : []}
      token={token}
      onRefetchTasks={onRefetchTasks}
      onRefetchArtifacts={onRefetchArtifacts}
      onViewArtifact={onViewArtifact}
    />
  )

  if (route.name === "workspace") return fallbackHome
  if (route.name === "activity") {
    return (
      <Activity
        workspaceId={route.workspaceId}
        tasks={workspaceTasks}
        artifacts={artifacts}
        getProjectName={(id) => getProjectName(projects, id)}
        onViewArtifact={onViewArtifact}
      />
    )
  }
  if (route.name === "explore") return <Explore workspaceId={route.workspaceId} />

  const project = "projectId" in route ? getProjectById(projects, route.projectId) : undefined
  const projectMismatch = !project || project.workspaceId !== route.workspaceId

  if (route.name === "project") {
    if (projectMismatch) return fallbackHome
    return fallbackProject(project)
  }
  if (route.name === "task") {
    const task = getTaskForDetail(tasks, workspaceTasks, route.projectId, route.taskId)
    if (!task) {
      if (projectMismatch) return fallbackHome
      return fallbackProject(project!)
    }
    return (
      <TaskDetail
        task={task}
        workspaceId={route.workspaceId}
        projectId={route.projectId}
        onRefetch={() => {
          onRefetchTasks()
          onRefetchWorkspaceTasks()
        }}
      />
    )
  }
  return fallbackHome
}
