import { useCallback, useEffect, useState } from "react"
import type { Artifact, Project, Task } from "../lib/types"
import type { Route } from "../lib/types"
import {
  getWorkspaces,
  getProjects,
  getTasks,
  getArtifacts,
  apiProjectToProject,
  apiTaskToTask,
  apiArtifactToArtifact,
  type ApiWorkspace,
} from "../lib/api"
import { useAsyncList } from "./useAsyncList"

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

export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const [loadingWorkspaces, setLoadingWorkspaces] = useState(true)
  const projectIdFromRoute = "projectId" in route ? route.projectId : undefined

  const { data: workspaces, refetch: refetchWorkspaces } = useAsyncList(
    () => getWorkspaces(token!),
    (x) => x,
    [token],
    !!token,
    { setLoading: setLoadingWorkspaces }
  )

  const { data: projects, refetch: refetchProjects } = useAsyncList(
    () => getProjects(route.workspaceId, token!),
    (list) => list.map(apiProjectToProject),
    [token, route.workspaceId],
    !!(token && route.workspaceId)
  )

  const { data: tasks, refetch: refetchTasks } = useAsyncList(
    () => getTasks(route.workspaceId, token!, projectIdFromRoute),
    (list) => list.map(apiTaskToTask),
    [token, route.workspaceId, projectIdFromRoute],
    !!(token && route.workspaceId && projectIdFromRoute)
  )

  const { data: workspaceTasks, refetch: refetchWorkspaceTasks } = useAsyncList(
    () => getTasks(route.workspaceId, token!),
    (list) => list.map(apiTaskToTask),
    [token, route.workspaceId],
    !!(token && route.workspaceId)
  )

  const [artifacts, setArtifacts] = useState<Artifact[]>([])
  const refetchArtifacts = useCallback(
    (projectId?: string, taskId?: string) => {
      if (!token || !route.workspaceId) return
      const options =
        projectId !== undefined || taskId !== undefined ? { projectId, taskId } : undefined
      getArtifacts(route.workspaceId, token, options)
        .then((list) => setArtifacts(list.map(apiArtifactToArtifact)))
        .catch(() => setArtifacts([]))
    },
    [token, route.workspaceId]
  )

  useEffect(() => {
    if (!token || !route.workspaceId) {
      setArtifacts([])
      return
    }
    getArtifacts(route.workspaceId, token, { projectId: projectIdFromRoute })
      .then((list) => setArtifacts(list.map(apiArtifactToArtifact)))
      .catch(() => setArtifacts([]))
  }, [token, route.workspaceId, projectIdFromRoute])

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