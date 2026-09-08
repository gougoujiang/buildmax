import { useAuth } from "../../contexts/AuthContext"
import {
  ConversationDetailView,
  useConversationDetail,
  useConversationTasks,
} from "../../features/conversations"
import { RunTraceModal } from "../../features/runs"
import { navigate } from "../../router"

interface ConversationDetailProps {
  spaceId: string
  conversationId: string
  onRefetch?: () => void
  initialMessage?: string
}

export function ConversationDetail({
  spaceId,
  conversationId,
  onRefetch,
  initialMessage,
}: ConversationDetailProps) {
  const { token, user } = useAuth()
  const taskCards = useConversationTasks({
    spaceId,
    conversationId,
    token,
  })
  const conversationDetail = useConversationDetail({
    spaceId,
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
        onOpenIssue={(issueId) => navigate({ name: "issue", spaceId, issueId })}
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
      <RunTraceModal
        open={taskCards.traceRunId != null}
        spaceId={spaceId}
        token={token}
        taskRunId={taskCards.traceRunId}
        onClose={taskCards.closeTrace}
      />
    </>
  )
}
