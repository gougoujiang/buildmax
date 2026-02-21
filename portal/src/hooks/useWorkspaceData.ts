import type { Artifact, Project, Route, Task } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { useWorkspaces } from "./useWorkspaces"
import { useProjects } from "./useProjects"
import { useTasks } from "./useTasks"
import { useWorkspaceTasks } from "./useWorkspaceTasks"
import { useArtifacts } from "./useArtifacts"

export interface UseWorkspaceDataResult {
  workspaces: ApiWorkspace[]
  projects: Project[]
  tasks: Task[]
  workspaceTasks: Task[]
  artifacts: Artifact[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => void
  refetchProjects: () => void
  refetchTasks: () => void
  refetchWorkspaceTasks: () => void
  refetchArtifacts: (projectId?: string, taskId?: string) => void
}

/** Composes useWorkspaces, useProjects, useTasks, useWorkspaceTasks, useArtifacts for the current route. */
export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const { data: workspaces, loading: loadingWorkspaces, refetch: refetchWorkspaces } = useWorkspaces(token)
  const { data: projects, refetch: refetchProjects } = useProjects(route.workspaceId, token)
  const { data: tasks, refetch: refetchTasks } = useTasks(
    route.workspaceId,
    token,
    undefined
  )
  const { data: workspaceTasks, refetch: refetchWorkspaceTasks } = useWorkspaceTasks(
    route.workspaceId,
    token
  )
  const { data: artifacts, refetch: artifactsRefetch } = useArtifacts(
    route.workspaceId,
    token,
    {}
  )

  const refetchArtifacts = (projectId?: string, taskId?: string) => {
    artifactsRefetch(
      projectId !== undefined || taskId !== undefined ? { projectId, taskId } : undefined
    )
  }

  return {
    workspaces,
    projects,
    tasks,
    workspaceTasks,
    artifacts,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchProjects,
    refetchTasks,
    refetchWorkspaceTasks,
    refetchArtifacts,
  }
}
