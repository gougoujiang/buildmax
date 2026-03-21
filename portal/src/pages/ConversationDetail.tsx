import { useAuth } from "../contexts/AuthContext"
import { ConversationDetailView, useConversationDetail } from "../features/conversations"

interface ConversationDetailProps {
  conversationId: string
  profileId: string
  onRefetch?: () => void
  initialMessage?: string
}

export function ConversationDetail({
  conversationId,
  profileId,
  onRefetch,
  initialMessage,
}: ConversationDetailProps) {
  const { token, user } = useAuth()
  const conversationDetail = useConversationDetail({
    profileId,
    conversationId,
    token,
    initialMessage,
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
      optimisticUserMessage={conversationDetail.optimisticUserMessage}
      user={user}
      onSend={conversationDetail.handleSend}
    />
  )
}
