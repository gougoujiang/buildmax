import { useAuth } from "../contexts/AuthContext"
import { ConversationDetailView, useConversationDetail } from "../features/conversations"

interface ConversationDetailProps {
  conversationId: string
  workspaceId: string
  onRefetch?: () => void
}

export function ConversationDetail({
  conversationId,
  workspaceId,
  onRefetch,
}: ConversationDetailProps) {
  const { token, user } = useAuth()
  const conversationDetail = useConversationDetail({
    workspaceId,
    conversationId,
    token,
    onMessageSent: onRefetch,
  })

  return (
    <ConversationDetailView
      historyRef={conversationDetail.historyRef}
      messages={conversationDetail.messages}
      messagesLoading={conversationDetail.messagesLoading}
      messagesError={conversationDetail.messagesError}
      input={conversationDetail.input}
      setInput={conversationDetail.setInput}
      sending={conversationDetail.sending}
      sendError={conversationDetail.sendError}
      streamingContent={conversationDetail.streamingContent}
      user={user}
      onSend={conversationDetail.handleSend}
    />
  )
}
