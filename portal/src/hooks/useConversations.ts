import type { Conversation } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { apiConversationToConversation } from "../lib/api/mappers"
import { getConversations } from "../features/conversations"
import { useAsyncList } from "./useAsyncList"

const CONVERSATIONS_LIMIT = 100

export function useConversations(
  token: string | null,
  currentTeamId: string | null,
  enabled = true
): { data: Conversation[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  return useAsyncList(
    () =>
      getConversations(currentTeamId!, token!, { limit: CONVERSATIONS_LIMIT }).then(
        (res) => res.conversations
      ),
    (list) => list.map(apiConversationToConversation),
    [token, currentTeamId],
    enabled && !!token,
    { errorMessage: (e) => getErrorMessage(e, "Failed to load conversations") }
  )
}
