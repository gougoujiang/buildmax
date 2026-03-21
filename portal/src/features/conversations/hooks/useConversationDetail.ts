import { useCallback, useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../../../lib/errorMessage"
import { useFetch } from "../../../hooks/useFetch"
import {
  addConversationMessageStream,
  getConversationMessages,
} from "../api"

interface UseConversationDetailOptions {
  profileId: string
  conversationId: string
  token: string | null
  initialMessage?: string
  onMessageSent?: () => void
}

export function useConversationDetail({
  profileId,
  conversationId,
  token,
  initialMessage,
  onMessageSent,
}: UseConversationDetailOptions) {
  const historyRef = useRef<HTMLElement | null>(null)
  const {
    data: messagesData,
    loading: messagesLoading,
    error: messagesError,
    refetch: refetchMessages,
  } = useFetch(
    () => getConversationMessages(profileId, conversationId, token!),
    [profileId, conversationId, token],
    {
      enabled: !!(token && profileId && conversationId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load messages"),
    }
  )

  const [input, setInput] = useState("")
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState<string | null>(null)
  const [optimisticUserMessage, setOptimisticUserMessage] = useState<string | null>(null)
  const initialMessageSentRef = useRef(false)

  const runMessageStream = useCallback(
    async (content: string, signal?: AbortSignal) => {
      if (!token) return
      setSending(true)
      setSendError(null)
      setStreamingContent("")
      try {
        await addConversationMessageStream(
          profileId,
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
              if (err.name === "AbortError") {
                setSending(false)
                setStreamingContent(null)
                setOptimisticUserMessage(null)
                return
              }
              setSendError(getErrorMessage(err, "Failed to send message"))
              setSending(false)
              setStreamingContent(null)
              setOptimisticUserMessage(null)
            },
          },
          { signal }
        )
      } catch (err) {
        if (err instanceof Error && err.name === "AbortError") {
          setSending(false)
          setStreamingContent(null)
          setOptimisticUserMessage(null)
          return
        }
        setSendError(getErrorMessage(err, "Failed to send message"))
        setSending(false)
        setStreamingContent(null)
        setOptimisticUserMessage(null)
      }
    },
    [profileId, conversationId, token, refetchMessages, onMessageSent]
  )

  const runMessageStreamRef = useRef(runMessageStream)
  runMessageStreamRef.current = runMessageStream

  // Send the initial message exactly once when navigating from "new conversation".
  useEffect(() => {
    if (!initialMessage || !token || initialMessageSentRef.current) return
    initialMessageSentRef.current = true
    setOptimisticUserMessage(initialMessage)
    void runMessageStreamRef.current(initialMessage)
  }, [initialMessage, token])

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
    await runMessageStream(content)
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
