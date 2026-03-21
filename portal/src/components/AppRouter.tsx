import { useEffect } from "react"
import type { Conversation, ViewArtifactParams } from "../lib/types"
import { getTaskForDetail } from "../lib/taskUtils"
import { useApp } from "../contexts/AppContext"
import { useArtifacts } from "../hooks/useArtifacts"
import { useTasks } from "../hooks/useTasks"
import { Tasks } from "../pages/Tasks"
import { AgentList } from "../pages/AgentList"
import { TaskDetail } from "../pages/TaskDetail"
import { ConversationDetail } from "../pages/ConversationDetail"
import { Home } from "../pages/Home"
import { NewConversation } from "../pages/NewConversation"
import { Explore } from "../pages/Explore"

export type { ViewArtifactParams }

export interface AppRouterProps {
  conversations: Conversation[]
  onRefetchConversations: () => Promise<void>
  onViewArtifact: (params: ViewArtifactParams) => void
}

export function AppRouter({
  conversations,
  onRefetchConversations,
  onViewArtifact,
}: AppRouterProps) {
  const {
    route,
    token,
    pendingTask,
    setPendingTask,
    pendingConversation,
    setPendingConversation,
  } = useApp()
  const isHome = route.name === "home"
  const isNewChat = route.name === "newChat"
  const isTaskDetail = route.name === "task"
  const {
    data: artifacts,
  } = useArtifacts(route.profileId, token, { enabled: isHome || isNewChat })
  const {
    data: tasks,
    refetch: refetchTasks,
  } = useTasks(route.profileId, token, route.name === "task" ? route.taskId : undefined, isTaskDetail)

  const routeTaskId = route.name === "task" ? route.taskId : undefined
  const routeConversationId = route.name === "conversation" ? route.conversationId : undefined

  useEffect(() => {
    if (!pendingTask) return
    const viewingThisTask = route.name === "task" && route.taskId === pendingTask.task.id
    if (!viewingThisTask) setPendingTask(null)
  }, [route.name, routeTaskId, pendingTask, setPendingTask])

  useEffect(() => {
    if (!pendingConversation) return
    const viewing = route.name === "conversation" && route.conversationId === pendingConversation.conversationId
    if (!viewing) setPendingConversation(null)
  }, [route.name, routeConversationId, pendingConversation, setPendingConversation])

  const fallbackHome = (
    <Home
      profileId={route.profileId}
      conversations={conversations}
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
          onRefetchConversations={onRefetchConversations}
          conversations={conversations}
          artifacts={artifacts}
          onViewArtifact={onViewArtifact}
      />
    )
  }
  if (route.name === "chats") {
    return (
      <Tasks
        profileId={route.profileId}
        conversations={conversations}
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

  if (route.name === "task") {
    const taskFromList = getTaskForDetail(tasks, route.taskId)
    const task = taskFromList ?? (pendingTask?.task.id === route.taskId ? pendingTask.task : null)
    if (!task) return fallbackHome
    const initialInput = pendingTask?.task.id === route.taskId ? pendingTask.initialInput : undefined
    return (
      <TaskDetail
        task={task}
        profileId={route.profileId}
        onRefetch={() => refetchTasks()}
        initialInput={initialInput}
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
          profileId={route.profileId}
          onRefetch={onRefetchConversations}
          initialMessage={initialMessage}
        />
      )
  }

  return fallbackHome
}
