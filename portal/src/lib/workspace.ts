import type { Chat } from "./types"

/** Resolve chat by id from workspace chat list (chats are workspace-scoped only). */
export function getChatForDetail(
  workspaceChats: Chat[],
  chatId: string
): Chat | undefined {
  return workspaceChats.find((c) => c.id === chatId)
}
