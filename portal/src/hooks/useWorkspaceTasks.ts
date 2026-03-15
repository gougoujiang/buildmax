import type { Chat } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { apiChatToChat } from "../lib/api"
import { getChat } from "../features/chats"
import { useFetch } from "./useFetch"

export function useWorkspaceChats(
  profileId: string,
  token: string | null,
  chatId?: string,
  enabled = true
): { data: Chat[]; loading: boolean; error: string | null; refetch: () => Promise<void> } {
  const result = useFetch(
    () => getChat(chatId!, token!),
    [token, chatId],
    {
      enabled: enabled && !!(token && profileId && chatId),
      errorMessage: (e) => getErrorMessage(e, "Failed to load chat"),
    }
  )
  return {
    data: result.data ? [apiChatToChat(result.data)] : [],
    loading: result.loading,
    error: result.error,
    refetch: async () => {
      result.refetch()
    },
  }
}
