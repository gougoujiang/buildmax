import { useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../../../lib/errorMessage"
import { useFetch } from "../../../hooks/useFetch"
import {
  addConversationMessageStream,
  getConversationMessages,
} from "../api"

interface UseConversationDetailOptions {
  workspaceId: string
  conversationId: string
  token: string | null
  onMessageSent?: () => void
}

export function useConversationDetail({
  workspaceId,
  conversationId,
  token,
  onMessageSent,
}: UseConversationDetailOptions) {
  const historyRef = useRef<HTMLElement | null>(null)
  const {
    data: messagesData,
    loading: messagesLoading,
    error: messagesError,
    refetch: refetchMessages,
  } = useFetch(
    () => getConversationMessages(workspaceId, conversationId, token!),
    [workspaceId, conversationId, token],
    {
      enabled: !!(token && workspaceId && conversationId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load messages"),
    }
  )

  const [input, setInput] = useState("")
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState<string | null>(null)

  useEffect(() => {
    const el = historyRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [messagesData?.messages, streamingContent])

  async function handleSend() {
    const content = input.trim()
    if (!content || !token || sending) return
    setSending(true)
    setSendError(null)
    setStreamingContent("")
    try {
      await addConversationMessageStream(
        workspaceId,
        conversationId,
        { content },
        token,
        {
          onDelta: (delta) => setStreamingContent((prev) => (prev ?? "") + delta),
          onDone: () => {
            setInput("")
            setSending(false)
            setStreamingContent(null)
            refetchMessages()
            onMessageSent?.()
          },
          onError: (err) => {
            setSendError(getErrorMessage(err, "Failed to send message"))
            setSending(false)
            setStreamingContent(null)
          },
        }
      )
    } catch (err) {
      setSendError(getErrorMessage(err, "Failed to send message"))
      setSending(false)
      setStreamingContent(null)
    }
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
    handleSend,
  }
}
