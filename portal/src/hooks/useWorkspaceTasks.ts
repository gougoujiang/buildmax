import type { Chat } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { getChats, apiChatToChat } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaceChats(
  workspaceId: string,
  token: string | null
): { data: Chat[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  return useAsyncList(
    () => getChats(workspaceId, token!),
    (list) => list.map(apiChatToChat),
    [token, workspaceId],
    !!(token && workspaceId),
    { errorMessage: (e) => getErrorMessage(e, "Failed to load chats") }
  )
}
