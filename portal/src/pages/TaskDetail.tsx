import type { Task } from "../lib/types"
import { useAuth } from "../contexts/AuthContext"
import { TaskDetailView, useTaskDetail } from "../features/tasks"

interface TaskDetailProps {
  task: Task
  profileId: string
  onRefetch?: () => void
  /** First user query when navigating before the task has messages. */
  initialInput?: string
}

export function TaskDetail({ task, profileId, onRefetch, initialInput }: TaskDetailProps) {
  const { token, user } = useAuth()
  const taskDetail = useTaskDetail({
    profileId,
    taskId: task.id,
    token,
    initialInput,
    onRunComplete: onRefetch,
  })

  return (
    <TaskDetailView
      historyRef={taskDetail.historyRef}
      session={taskDetail.session}
      sessionLoading={taskDetail.sessionLoading}
      sessionError={taskDetail.sessionError}
      followUpInput={taskDetail.followUpInput}
      setFollowUpInput={taskDetail.setFollowUpInput}
      followUpLoading={taskDetail.followUpLoading}
      followUpError={taskDetail.followUpError}
      streamingContent={taskDetail.streamingContent}
      lastSentMessage={taskDetail.lastSentMessage}
      user={user}
      initialInput={initialInput}
      showInitialInput={taskDetail.showInitialInput}
      expandedToolIndices={taskDetail.expandedToolIndices}
      toggleToolExpand={taskDetail.toggleToolExpand}
      onSubmitFollowUp={taskDetail.submitFollowUp}
    />
  )
}
