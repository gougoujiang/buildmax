import type { Artifact, Route, Chat } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { useWorkspaces } from "./useWorkspaces"
import { useWorkspaceChats } from "./useWorkspaceTasks"
import { useArtifacts } from "./useArtifacts"

export interface UseWorkspaceDataResult {
  workspaces: ApiWorkspace[]
  workspaceChats: Chat[]
  artifacts: Artifact[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => Promise<void>
  refetchWorkspaceChats: () => Promise<void>
  refetchArtifacts: (chatId?: string) => void
}

/** Composes useWorkspaces, useWorkspaceChats, useArtifacts for the current route. */
export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const { data: workspaces, loading: loadingWorkspaces, refetch: refetchWorkspaces } = useWorkspaces(token)
  const { data: workspaceChats, refetch: refetchWorkspaceChats } = useWorkspaceChats(
    route.workspaceId,
    token
  )
  const { data: artifacts, refetch: artifactsRefetch } = useArtifacts(
    route.workspaceId,
    token,
    {}
  )

  const refetchArtifacts = (chatId?: string) => {
    artifactsRefetch(chatId !== undefined ? { chatId } : undefined)
  }

  return {
    workspaces,
    workspaceChats,
    artifacts,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchWorkspaceChats,
    refetchArtifacts,
  }
}
