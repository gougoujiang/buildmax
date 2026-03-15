import type { Chat } from "./types"

/** Resolve chat by id from the currently loaded chat list. */
export function getChatForDetail(
  workspaceChats: Chat[],
  chatId: string
): Chat | undefined {
  return workspaceChats.find((c) => c.id === chatId)
}
