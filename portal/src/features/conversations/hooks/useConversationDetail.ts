import { useCallback, useEffect, useRef, useState } from "react"
import { useFetch } from "../../../hooks/useFetch"
import { getConversationMessages } from "../api"
import { useWebSocket } from "../../../contexts/WebSocketContext"

interface UseConversationDetailOptions {
  conversationId: string
  token: string | null
  initialMessage?: string
  onMessageSent?: () => void
}

interface MessageDeltaPayload {
  conversation_id: string
  delta: string
}

interface MessageCompletedPayload {
  conversation_id: string
}

interface ConversationErrorPayload {
  conversation_id?: string
  error: string
}

export function useConversationDetail({
  conversationId,
  token,
  initialMessage,
  onMessageSent,
}: UseConversationDetailOptions) {
  const historyRef = useRef<HTMLElement | null>(null)
  const ws = useWebSocket()
  const {
    data: messagesData,
    loading: messagesLoading,
    error: messagesError,
    refetch: refetchMessages,
  } = useFetch(
    () => getConversationMessages(conversationId, token!),
    [conversationId, token],
    {
      enabled: !!(token && conversationId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load messages"),
    }
  )

  const [input, setInput] = useState("")
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState<string | null>(null)
  const [optimisticUserMessage, setOptimisticUserMessage] = useState<string | null>(null)
  const initialMessageSentRef = useRef(false)

  const refetchMessagesRef = useRef(refetchMessages)
  refetchMessagesRef.current = refetchMessages
  const onMessageSentRef = useRef(onMessageSent)
  onMessageSentRef.current = onMessageSent

  useEffect(() => {
    const handleDelta = (payload: MessageDeltaPayload) => {
      if (payload.conversation_id !== conversationId) return
      setStreamingContent((prev) => (prev ?? "") + payload.delta)
    }

    const handleCompleted = (payload: MessageCompletedPayload) => {
      if (payload.conversation_id !== conversationId) return
      setInput("")
      setSending(false)
      setStreamingContent(null)
      refetchMessagesRef.current()
      onMessageSentRef.current?.()
    }

    const handleError = (payload: ConversationErrorPayload) => {
      if (payload.conversation_id && payload.conversation_id !== conversationId) return
      setSendError(payload.error)
      setSending(false)
      setStreamingContent(null)
      setOptimisticUserMessage(null)
    }

    ws.on("conversation.message.delta", handleDelta)
    ws.on("conversation.message.completed", handleCompleted)
    ws.on("conversation.error", handleError)

    return () => {
      ws.off("conversation.message.delta", handleDelta)
      ws.off("conversation.message.completed", handleCompleted)
      ws.off("conversation.error", handleError)
    }
  }, [ws, conversationId])

  const sendMessage = useCallback(
    (content: string) => {
      if (!token) return
      setSending(true)
      setSendError(null)
      setStreamingContent("")
      setOptimisticUserMessage(content)
      ws.send("conversation.message", {
        conversation_id: conversationId,
        content,
      })
    },
    [ws, conversationId, token]
  )

  // Send the initial message exactly once when navigating from "new conversation".
  useEffect(() => {
    if (!initialMessage || !token || initialMessageSentRef.current) return
    initialMessageSentRef.current = true
    sendMessage(initialMessage)
  }, [initialMessage, token, sendMessage])

  // Clear optimistic bubble once the real message list includes it.
  useEffect(() => {
    const msgs = messagesData?.messages
    if (!optimisticUserMessage || !msgs?.length) return
    if (msgs.some((m) => m.role === "user" && m.content === optimisticUserMessage)) {
      setOptimisticUserMessage(null)
    }
  }, [messagesData?.messages, optimisticUserMessage])

  useEffect(() => {
    const el = historyRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [messagesData?.messages, streamingContent, optimisticUserMessage])

  async function handleSend() {
    const content = input.trim()
    if (!content || !token || sending) return
    sendMessage(content)
  }

  return {
    historyRef,
    messages: messagesData?.messages ?? [],
    messagesLoading,
    messagesError,
    input,
    setInput,
    sending,
    sendError,
    streamingContent,
    optimisticUserMessage,
    handleSend,
  }
}
