import type { Chat } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { apiChatToChat } from "../lib/api"
import { getChats } from "../features/chats"
import { useAsyncList } from "./useAsyncList"

export function useWorkspaceChats(
  workspaceId: string,
  token: string | null,
  enabled = true
): { data: Chat[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  return useAsyncList(
    () => getChats(workspaceId, token!),
    (list) => list.map(apiChatToChat),
    [token, workspaceId],
    enabled && !!(token && workspaceId),
    { errorMessage: (e) => getErrorMessage(e, "Failed to load chats") }
  )
}
