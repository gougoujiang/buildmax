import { useCallback, useEffect, useRef, useState } from "react"
import { useFetch } from "../../../hooks/useFetch"
import { getConversationMessages } from "../api"
import {
  useConversationBusy,
  useWebSocket,
} from "../../../contexts/WebSocketContext"
import { useTeam } from "../../../contexts/TeamContext"

interface UseConversationDetailOptions {
  teamId: string | null
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
  teamId,
  conversationId,
  token,
  initialMessage,
  onMessageSent,
}: UseConversationDetailOptions) {
  const historyRef = useRef<HTMLElement | null>(null)
  const ws = useWebSocket()
  const { busy: sending, markBusy } = useConversationBusy(conversationId)
  const { currentTeamId } = useTeam()
  const {
    data: messagesData,
    loading: messagesLoading,
    error: messagesError,
    refetch: refetchMessages,
  } = useFetch(
    () => getConversationMessages(teamId!, conversationId, token!),
    [teamId, conversationId, token],
    {
      enabled: !!(token && teamId && conversationId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load messages"),
    }
  )

  const [input, setInput] = useState("")
  const [sendError, setSendError] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState<string | null>(null)
  const [optimisticUserMessage, setOptimisticUserMessage] = useState<string | null>(null)
  // Records the conversationId we've already sent the initial message for.
  // Keying by conversationId prevents an async team-id resolve (or any unrelated rerun
  // of the reset effect below) from wiping the guard and causing a duplicate send.
  const initialMessageSentForRef = useRef<string | null>(null)

  const refetchMessagesRef = useRef(refetchMessages)
  refetchMessagesRef.current = refetchMessages
  const onMessageSentRef = useRef(onMessageSent)
  onMessageSentRef.current = onMessageSent

  useEffect(() => {
    setInput("")
    setSendError(null)
    setStreamingContent(null)
    setOptimisticUserMessage(null)
  }, [conversationId, currentTeamId])

  useEffect(() => {
    const handleDelta = (payload: MessageDeltaPayload) => {
      if (payload.conversation_id !== conversationId) return
      setStreamingContent((prev) => (prev ?? "") + payload.delta)
    }

    const handleCompleted = (payload: MessageCompletedPayload) => {
      if (payload.conversation_id !== conversationId) return
      setInput("")
      setStreamingContent(null)
      refetchMessagesRef.current()
      onMessageSentRef.current?.()
    }

    const handleError = (payload: ConversationErrorPayload) => {
      if (payload.conversation_id && payload.conversation_id !== conversationId) return
      setSendError(payload.error)
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
      markBusy()
      setSendError(null)
      setStreamingContent("")
      setOptimisticUserMessage(content)
      ws.send("conversation.message", {
        conversation_id: conversationId,
        content,
      })
    },
    [ws, conversationId, token, markBusy]
  )

  // Send the initial message exactly once per conversation. Guarding by conversationId
  // (not a boolean reset by another effect) means a rerender from an unrelated context
  // change cannot cause a duplicate send.
  useEffect(() => {
    if (!initialMessage || !token) return
    if (initialMessageSentForRef.current === conversationId) return
    initialMessageSentForRef.current = conversationId
    sendMessage(initialMessage)
  }, [initialMessage, token, sendMessage, conversationId])

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
