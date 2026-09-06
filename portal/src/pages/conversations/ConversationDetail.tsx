import { useAuth } from "../../contexts/AuthContext"
import { useSpace } from "../../contexts/SpaceContext"
import {
  ConversationDetailView,
  TaskFilesModal,
  useConversationDetail,
  useConversationTasks,
} from "../../features/conversations"
import { RunTraceModal } from "../../features/runs"
import { navigate } from "../../router"

interface ConversationDetailProps {
  conversationId: string
  onRefetch?: () => void
  initialMessage?: string
}

export function ConversationDetail({
  conversationId,
  onRefetch,
  initialMessage,
}: ConversationDetailProps) {
  const { token, user } = useAuth()
  const { currentSpaceId } = useSpace()
  const taskCards = useConversationTasks({
    spaceId: currentSpaceId,
    conversationId,
    token,
  })
  const conversationDetail = useConversationDetail({
    spaceId: currentSpaceId,
    conversationId,
    token,
    initialMessage,
    onMessageSent: onRefetch,
    scrollSignal: taskCards.tasks,
  })

  return (
    <>
      <ConversationDetailView
        historyRef={conversationDetail.historyRef}
        messages={conversationDetail.messages}
        messagesLoading={conversationDetail.messagesLoading}
        messagesError={conversationDetail.messagesError}
        taskCards={taskCards}
        onOpenIssue={(issueId) => navigate({ name: "issue", issueId })}
        input={conversationDetail.input}
        setInput={conversationDetail.setInput}
        sending={conversationDetail.sending}
        sendError={conversationDetail.sendError}
        streamingContent={conversationDetail.streamingContent}
        optimisticUserMessage={conversationDetail.optimisticUserMessage}
        queuedMessages={conversationDetail.queuedMessages}
        user={user}
        onSend={conversationDetail.handleSend}
      />
      <TaskFilesModal
        open={taskCards.filesRunId != null}
        spaceId={currentSpaceId}
        token={token}
        taskRunId={taskCards.filesRunId}
        onClose={taskCards.closeFiles}
      />
      <RunTraceModal
        open={taskCards.traceRunId != null}
        spaceId={currentSpaceId}
        token={token}
        taskRunId={taskCards.traceRunId}
        onClose={taskCards.closeTrace}
      />
    </>
  )
}
