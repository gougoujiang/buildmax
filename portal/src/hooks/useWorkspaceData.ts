import type { Artifact, Route, Task } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { useWorkspaces } from "./useWorkspaces"
import { useWorkspaceTasks } from "./useWorkspaceTasks"
import { useArtifacts } from "./useArtifacts"

export interface UseWorkspaceDataResult {
  workspaces: ApiWorkspace[]
  workspaceTasks: Task[]
  artifacts: Artifact[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => void
  refetchWorkspaceTasks: () => void
  refetchArtifacts: (taskId?: string) => void
}

/** Composes useWorkspaces, useWorkspaceTasks, useArtifacts for the current route. */
export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const { data: workspaces, loading: loadingWorkspaces, refetch: refetchWorkspaces } = useWorkspaces(token)
  const { data: workspaceTasks, refetch: refetchWorkspaceTasks } = useWorkspaceTasks(
    route.workspaceId,
    token
  )
  const { data: artifacts, refetch: artifactsRefetch } = useArtifacts(
    route.workspaceId,
    token,
    {}
  )

  const refetchArtifacts = (taskId?: string) => {
    artifactsRefetch(taskId !== undefined ? { taskId } : undefined)
  }

  return {
    workspaces,
    workspaceTasks,
    artifacts,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchWorkspaceTasks,
    refetchArtifacts,
  }
}
