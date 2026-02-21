import type { Chat } from "../lib/types"
import { getChats, apiChatToChat } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaceChats(
  workspaceId: string,
  token: string | null
): { data: Chat[]; refetch: () => void } {
  return useAsyncList(
    () => getChats(workspaceId, token!),
    (list) => list.map(apiChatToChat),
    [token, workspaceId],
    !!(token && workspaceId)
  )
}
