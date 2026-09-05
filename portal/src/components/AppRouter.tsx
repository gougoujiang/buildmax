import { useEffect } from "react"
import type { Conversation } from "../lib/types"
import { useApp } from "../contexts/AppContext"
import { AgentList } from "../pages/agents/AgentList"
import { AgentDetail } from "../pages/agents/AgentDetail"
import { ConversationDetail } from "../pages/conversations/ConversationDetail"
import { TaskDetail } from "../pages/tasks/TaskDetail"
import { NewConversation } from "../pages/conversations/NewConversation"
import { Explore } from "../pages/explore/Explore"
import { Issues } from "../pages/issues/Issues"
import { IssueDetail } from "../pages/issues/IssueDetail"
import { Artifacts } from "../pages/artifacts/Artifacts"
import { ArtifactDetail } from "../pages/artifacts/ArtifactDetail"
import { Workflows } from "../pages/workflows/Workflows"
import { WorkflowDetail } from "../pages/workflows/WorkflowDetail"
import { WorkflowRunDetail } from "../pages/workflows/WorkflowRunDetail"
import { AccountSettings } from "../pages/settings/AccountSettings"
import { SpaceSettings } from "../pages/settings/SpaceSettings"
import { AdminSettings } from "../pages/admin/AdminSettings"

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
    const viewing = route.name === "conversation" && routeConversationId === pendingConversation.conversationId
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

  if (route.name === "agent") {
    return <AgentDetail token={token ?? null} agentId={route.agentId} />
  }

  if (route.name === "account") return <AccountSettings section={route.section ?? "general"} />
  if (route.name === "space") return <SpaceSettings section={route.section ?? "overview"} />
  if (route.name === "admin") return <AdminSettings section={route.section ?? "overview"} />

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

  if (route.name === "artifacts") {
    return <Artifacts />
  }

  if (route.name === "artifact") {
    return <ArtifactDetail artifactId={route.artifactId} />
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

  if (route.name === "task") {
    return <TaskDetail token={token ?? null} taskId={route.taskId} />
  }

  return fallbackHome
}
