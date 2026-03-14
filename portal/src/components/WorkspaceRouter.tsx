import { useEffect } from "react"
import type { Conversation, ViewArtifactParams } from "../lib/types"
import { getChatForDetail } from "../lib/workspace"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { useArtifacts } from "../hooks/useArtifacts"
import { useWorkspaceChats } from "../hooks/useWorkspaceTasks"
import { Chats } from "../pages/Chats"
import { AgentList } from "../pages/AgentList"
import { ChatDetail } from "../pages/ChatDetail"
import { ConversationDetail } from "../pages/ConversationDetail"
import { WorkspaceHome } from "../pages/WorkspaceHome"
import { NewChat } from "../pages/NewChat"
import { Explore } from "../pages/Explore"

export type { ViewArtifactParams }

export interface WorkspaceRouterProps {
  workspaceConversations: Conversation[]
  onRefetchWorkspaceConversations: () => Promise<void>
  onViewArtifact: (params: ViewArtifactParams) => void
}

export function WorkspaceRouter({
  workspaceConversations,
  onRefetchWorkspaceConversations,
  onViewArtifact,
}: WorkspaceRouterProps) {
  const {
    route,
    token,
    pendingChat,
    setPendingChat,
  } = useWorkspace()
  const isWorkspaceHome = route.name === "workspace"
  const isNewChat = route.name === "newChat"
  const isChatDetail = route.name === "chat"
  const {
    data: artifacts,
  } = useArtifacts(route.workspaceId, token, { enabled: isWorkspaceHome || isNewChat })
  const {
    data: workspaceChats,
    refetch: refetchWorkspaceChats,
  } = useWorkspaceChats(route.workspaceId, token, isChatDetail)

  const routeChatId = route.name === "chat" ? route.chatId : undefined
  // Clear pending chat only when navigating away from this chat, so initialInput stays visible until we leave.
  useEffect(() => {
    if (!pendingChat) return
    const viewingThisChat = route.name === "chat" && route.chatId === pendingChat.chat.id
    if (!viewingThisChat) setPendingChat(null)
  }, [route.name, routeChatId, pendingChat, setPendingChat])

  const fallbackHome = (
    <WorkspaceHome
      workspaceId={route.workspaceId}
      workspaceConversations={workspaceConversations}
      artifacts={artifacts}
      onViewArtifact={onViewArtifact}
    />
  )

  if (route.name === "workspace") return fallbackHome
  if (route.name === "newChat") {
    return (
        <NewChat
          workspaceId={route.workspaceId}
          token={token ?? undefined}
          onRefetchWorkspaceConversations={onRefetchWorkspaceConversations}
          workspaceConversations={workspaceConversations}
          artifacts={artifacts}
          onViewArtifact={onViewArtifact}
      />
    )
  }
  if (route.name === "chats") {
    return (
      <Chats
        workspaceId={route.workspaceId}
        conversations={workspaceConversations}
      />
    )
  }
  if (route.name === "explore") return <Explore workspaceId={route.workspaceId} />

  if (route.name === "agents") {
    return (
      <AgentList
        workspaceId={route.workspaceId}
        token={token ?? null}
      />
    )
  }

  if (route.name === "chat") {
    const chatFromList = getChatForDetail(workspaceChats, route.chatId)
    const chat = chatFromList ?? (pendingChat?.chat.id === route.chatId ? pendingChat.chat : null)
    if (!chat) return fallbackHome
    const initialInput = pendingChat?.chat.id === route.chatId ? pendingChat.initialInput : undefined
    return (
      <ChatDetail
        chat={chat}
        workspaceId={route.workspaceId}
        onRefetch={() => refetchWorkspaceChats()}
        initialInput={initialInput}
      />
    )
  }

  if (route.name === "conversation") {
    return (
        <ConversationDetail
          conversationId={route.conversationId}
          workspaceId={route.workspaceId}
          onRefetch={onRefetchWorkspaceConversations}
        />
      )
  }

  return fallbackHome
}
