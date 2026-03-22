import { useEffect } from "react"
import type { Conversation } from "../lib/types"
import { useApp } from "../contexts/AppContext"
import { Tasks } from "../pages/Tasks"
import { AgentList } from "../pages/AgentList"
import { ConversationDetail } from "../pages/ConversationDetail"
import { Home } from "../pages/Home"
import { NewConversation } from "../pages/NewConversation"
import { Explore } from "../pages/Explore"

export interface AppRouterProps {
  conversations: Conversation[]
  onRefetchConversations: () => Promise<void>
}

export function AppRouter({
  conversations,
  onRefetchConversations,
}: AppRouterProps) {
  const {
    route,
    token,
    pendingConversation,
    setPendingConversation,
  } = useApp()

  const routeConversationId = route.name === "conversation" ? route.conversationId : undefined

  useEffect(() => {
    if (!pendingConversation) return
    const viewing = route.name === "conversation" && route.conversationId === pendingConversation.conversationId
    if (!viewing) setPendingConversation(null)
  }, [route.name, routeConversationId, pendingConversation, setPendingConversation])

  const fallbackHome = (
    <Home
      conversations={conversations}
    />
  )

  if (route.name === "home") return fallbackHome
  if (route.name === "newChat") {
    return (
        <NewConversation
          token={token ?? undefined}
          onRefetchConversations={onRefetchConversations}
          conversations={conversations}
      />
    )
  }
  if (route.name === "chats") {
    return (
      <Tasks
        conversations={conversations}
      />
    )
  }
  if (route.name === "explore") return <Explore />

  if (route.name === "agents") {
    return (
      <AgentList
        token={token ?? null}
      />
    )
  }

  if (route.name === "conversation") {
    const initialMessage =
      pendingConversation?.conversationId === route.conversationId
        ? pendingConversation.initialMessage
        : undefined
    return (
        <ConversationDetail
          conversationId={route.conversationId}
          onRefetch={onRefetchConversations}
          initialMessage={initialMessage}
        />
      )
  }

  return fallbackHome
}
