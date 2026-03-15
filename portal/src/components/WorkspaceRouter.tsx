import { useEffect } from "react"
import type { Conversation, ViewArtifactParams } from "../lib/types"
import { getChatForDetail } from "../lib/workspace"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { useArtifacts } from "../hooks/useArtifacts"
import { useWorkspaceChats } from "../hooks/useWorkspaceTasks"
import { Tasks } from "../pages/Tasks"
import { AgentList } from "../pages/AgentList"
import { TaskDetail } from "../pages/TaskDetail"
import { ConversationDetail } from "../pages/ConversationDetail"
import { WorkspaceHome } from "../pages/WorkspaceHome"
import { NewConversation } from "../pages/NewConversation"
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
  const isWorkspaceHome = route.name === "home"
  const isNewChat = route.name === "newChat"
  const isChatDetail = route.name === "chat"
  const {
    data: artifacts,
  } = useArtifacts(route.profileId, token, { enabled: isWorkspaceHome || isNewChat })
  const {
    data: workspaceChats,
    refetch: refetchWorkspaceChats,
  } = useWorkspaceChats(route.profileId, token, route.name === "chat" ? route.chatId : undefined, isChatDetail)

  const routeChatId = route.name === "chat" ? route.chatId : undefined
  // Clear pending chat only when navigating away from this chat, so initialInput stays visible until we leave.
  useEffect(() => {
    if (!pendingChat) return
    const viewingThisChat = route.name === "chat" && route.chatId === pendingChat.chat.id
    if (!viewingThisChat) setPendingChat(null)
  }, [route.name, routeChatId, pendingChat, setPendingChat])

  const fallbackHome = (
    <WorkspaceHome
      profileId={route.profileId}
      workspaceConversations={workspaceConversations}
      artifacts={artifacts}
      onViewArtifact={onViewArtifact}
    />
  )

  if (route.name === "home") return fallbackHome
  if (route.name === "newChat") {
    return (
        <NewConversation
          profileId={route.profileId}
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
      <Tasks
        profileId={route.profileId}
        conversations={workspaceConversations}
      />
    )
  }
  if (route.name === "explore") return <Explore profileId={route.profileId} />

  if (route.name === "agents") {
    return (
      <AgentList
        profileId={route.profileId}
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
      <TaskDetail
        chat={chat}
        profileId={route.profileId}
        onRefetch={() => refetchWorkspaceChats()}
        initialInput={initialInput}
      />
    )
  }

  if (route.name === "conversation") {
    return (
        <ConversationDetail
          conversationId={route.conversationId}
          profileId={route.profileId}
          onRefetch={onRefetchWorkspaceConversations}
        />
      )
  }

  return fallbackHome
}
