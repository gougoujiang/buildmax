import type { Artifact, Route, Chat, Conversation } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { useWorkspaces } from "./useWorkspaces"
import { useWorkspaceChats } from "./useWorkspaceTasks"
import { useWorkspaceConversations } from "./useWorkspaceConversations"
import { useArtifacts } from "./useArtifacts"

export interface UseWorkspaceDataResult {
  workspaces: ApiWorkspace[]
  workspaceChats: Chat[]
  workspaceConversations: Conversation[]
  artifacts: Artifact[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => Promise<void>
  refetchWorkspaceChats: () => Promise<void>
  refetchWorkspaceConversations: () => Promise<void>
  refetchArtifacts: (chatId?: string) => void
}

/** Composes useWorkspaces, useWorkspaceChats, useWorkspaceConversations, useArtifacts for the current route. */
export function useWorkspaceData(
  token: string | null,
  route: Route
): UseWorkspaceDataResult {
  const { data: workspaces, loading: loadingWorkspaces, refetch: refetchWorkspaces } = useWorkspaces(token)
  const { data: workspaceChats, refetch: refetchWorkspaceChats } = useWorkspaceChats(
    route.workspaceId,
    token
  )
  const { data: workspaceConversations, refetch: refetchWorkspaceConversations } =
    useWorkspaceConversations(route.workspaceId, token)
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
    workspaceConversations,
    artifacts,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchWorkspaceChats,
    refetchWorkspaceConversations,
    refetchArtifacts,
  }
}
