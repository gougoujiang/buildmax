import type { ViewArtifactParams } from "../lib/types"
import { getChatForDetail } from "../lib/workspace"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { Chats } from "../pages/Chats"
import { AgentList } from "../pages/AgentList"
import { TaskDetail } from "../pages/TaskDetail"
import { WorkspaceHome } from "../pages/WorkspaceHome"
import { NewChat } from "../pages/NewChat"
import { Explore } from "../pages/Explore"

export type { ViewArtifactParams }

export interface WorkspaceRouterProps {
  onViewArtifact: (params: ViewArtifactParams) => void
}

export function WorkspaceRouter({ onViewArtifact }: WorkspaceRouterProps) {
  const {
    route,
    workspaceChats,
    artifacts,
    token,
    refetchWorkspaceChats,
  } = useWorkspace()

  const fallbackHome = (
    <WorkspaceHome
      workspaceId={route.workspaceId}
      workspaceChats={workspaceChats}
      artifacts={artifacts}
      token={token ?? undefined}
      onRefetchWorkspaceChats={refetchWorkspaceChats}
      onViewArtifact={onViewArtifact}
    />
  )

  if (route.name === "workspace") return fallbackHome
  if (route.name === "newChat") {
    return (
      <NewChat
        workspaceId={route.workspaceId}
        token={token ?? undefined}
        onRefetchWorkspaceChats={refetchWorkspaceChats}
      />
    )
  }
  if (route.name === "chats") {
    return (
      <Chats
        workspaceId={route.workspaceId}
        chats={workspaceChats}
        artifacts={artifacts}
        onViewArtifact={onViewArtifact}
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
    const chat = getChatForDetail(workspaceChats, route.chatId)
    if (!chat) return fallbackHome
    return (
      <TaskDetail
        chat={chat}
        workspaceId={route.workspaceId}
        onRefetch={() => refetchWorkspaceChats()}
      />
    )
  }

  return fallbackHome
}
