import type { ViewArtifactParams } from "../lib/types"
import { getTaskForDetail } from "../lib/workspace"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { Activity } from "../pages/Activity"
import { Agents } from "../pages/Agents"
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
    workspaceTasks,
    artifacts,
    token,
    refetchWorkspaceTasks,
  } = useWorkspace()

  const fallbackHome = (
    <WorkspaceHome
      workspaceId={route.workspaceId}
      workspaceTasks={workspaceTasks}
      artifacts={artifacts}
      token={token ?? undefined}
      onRefetchWorkspaceTasks={refetchWorkspaceTasks}
      onViewArtifact={onViewArtifact}
    />
  )

  if (route.name === "workspace") return fallbackHome
  if (route.name === "newChat") {
    return (
      <NewChat
        workspaceId={route.workspaceId}
        token={token ?? undefined}
        onRefetchWorkspaceTasks={refetchWorkspaceTasks}
      />
    )
  }
  if (route.name === "activity") {
    return (
      <Activity
        workspaceId={route.workspaceId}
        tasks={workspaceTasks}
        artifacts={artifacts}
        onViewArtifact={onViewArtifact}
      />
    )
  }
  if (route.name === "explore") return <Explore workspaceId={route.workspaceId} />

  if (route.name === "agents") {
    return (
      <Agents
        workspaceId={route.workspaceId}
        token={token ?? null}
        onViewArtifact={onViewArtifact}
      />
    )
  }
  if (route.name === "agentList") {
    return (
      <AgentList
        workspaceId={route.workspaceId}
        token={token ?? null}
      />
    )
  }

  if (route.name === "task") {
    const task = getTaskForDetail(workspaceTasks, route.taskId)
    if (!task) return fallbackHome
    return (
      <TaskDetail
        task={task}
        workspaceId={route.workspaceId}
        onRefetch={() => refetchWorkspaceTasks()}
      />
    )
  }

  return fallbackHome
}
