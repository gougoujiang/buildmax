import { useEffect } from "react"
import type { Conversation } from "../lib/types"
import { useApp } from "../contexts/AppContext"
import { AgentList } from "../pages/AgentList"
import { ConversationDetail } from "../pages/ConversationDetail"
import { NewConversation } from "../pages/NewConversation"
import { Explore } from "../pages/Explore"
import { Issues } from "../pages/Issues"
import { IssueDetail } from "../pages/IssueDetail"
import { Workflows } from "../pages/Workflows"
import { WorkflowDetail } from "../pages/WorkflowDetail"
import { WorkflowRunDetail } from "../pages/WorkflowRunDetail"
import { AccountSettings } from "../pages/AccountSettings"
import { SpaceSettings } from "../pages/SpaceSettings"

export interface AppRouterProps {
  conversations: Conversation[]
  onRefetchConversations: () => Promise<void>
  userId: string
}

export function AppRouter({
  conversations,
  onRefetchConversations,
  userId,
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
    <NewConversation
      token={token ?? undefined}
      onRefetchConversations={onRefetchConversations}
      conversations={conversations}
    />
  )

  if (route.name === "home") return fallbackHome
  if (route.name === "conversations") return fallbackHome
  if (route.name === "explore") return <Explore />

  if (route.name === "agents") {
    return (
      <AgentList
        token={token ?? null}
      />
    )
  }

  if (route.name === "account") return <AccountSettings section={route.section ?? "general"} />
  if (route.name === "space") return <SpaceSettings section={route.section ?? "overview"} />

  if (route.name === "workflows") {
    return <Workflows token={token ?? null} />
  }

  if (route.name === "workflow") {
    return <WorkflowDetail token={token ?? null} workflowId={route.workflowId} />
  }

  if (route.name === "workflowRun") {
    return <WorkflowRunDetail token={token ?? null} workflowRunId={route.workflowRunId} />
  }

  if (route.name === "issues") {
    return <Issues token={token ?? null} userId={userId} />
  }

  if (route.name === "issue") {
    return <IssueDetail token={token ?? null} issueId={route.issueId} userId={userId} />
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
