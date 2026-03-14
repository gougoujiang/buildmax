import type { Conversation } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { getConversations, apiConversationToConversation } from "../lib/api"
import { useAsyncList } from "./useAsyncList"

const CONVERSATIONS_LIMIT = 100

export function useWorkspaceConversations(
  workspaceId: string,
  token: string | null
): { data: Conversation[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  return useAsyncList(
    () =>
      getConversations(workspaceId, token!, { limit: CONVERSATIONS_LIMIT }).then(
        (res) => res.conversations
      ),
    (list) => list.map(apiConversationToConversation),
    [token, workspaceId],
    !!(token && workspaceId),
    { errorMessage: (e) => getErrorMessage(e, "Failed to load conversations") }
  )
}
