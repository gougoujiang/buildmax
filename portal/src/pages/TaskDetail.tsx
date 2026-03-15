import type { Chat } from "../lib/types"
import { useAuth } from "../contexts/AuthContext"
import { ChatDetailView, useChatDetail } from "../features/chats"

interface TaskDetailProps {
  chat: Chat
  workspaceId: string
  onRefetch?: () => void
  /** First user query when navigating from New Conversation before the task has messages. */
  initialInput?: string
}

export function TaskDetail({ chat, workspaceId, onRefetch, initialInput }: TaskDetailProps) {
  const { token, user } = useAuth()
  const chatDetail = useChatDetail({
    workspaceId,
    chatId: chat.id,
    token,
    initialInput,
    onRunComplete: onRefetch,
  })

  return (
    <ChatDetailView
      historyRef={chatDetail.historyRef}
      session={chatDetail.session}
      sessionLoading={chatDetail.sessionLoading}
      sessionError={chatDetail.sessionError}
      followUpInput={chatDetail.followUpInput}
      setFollowUpInput={chatDetail.setFollowUpInput}
      followUpLoading={chatDetail.followUpLoading}
      followUpError={chatDetail.followUpError}
      streamingContent={chatDetail.streamingContent}
      lastSentMessage={chatDetail.lastSentMessage}
      user={user}
      initialInput={initialInput}
      showInitialInput={chatDetail.showInitialInput}
      expandedToolIndices={chatDetail.expandedToolIndices}
      toggleToolExpand={chatDetail.toggleToolExpand}
      onSubmitFollowUp={chatDetail.submitFollowUp}
    />
  )
}
