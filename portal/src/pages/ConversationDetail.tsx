import { useEffect, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { getConversationMessages, addConversationMessageStream } from "../lib/api"
import { getErrorMessage } from "../lib/errorMessage"
import { useAuth } from "../contexts/AuthContext"
import { useFetch } from "../hooks/useFetch"
import type { ApiConversationMessage } from "../lib/api"

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
  const { token } = useAuth()
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
            onRefetch?.()
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

  const messages: ApiConversationMessage[] = messagesData?.messages ?? []

  return (
    <div className="page-chat">
      <section
        ref={historyRef}
        className="page-chat__history"
        aria-label="Conversation history"
      >
        {messagesLoading && messages.length === 0 ? (
          <p className="page-chat__text">Loading…</p>
        ) : messagesError ? (
          <p className="page-chat__text page-chat__error" role="alert">
            {messagesError}
          </p>
        ) : messages.length === 0 ? (
          <p className="page-chat__text page-chat__muted">No messages yet. Send one below.</p>
        ) : (
          <ul className="page-chat__message-list">
            {messages.map((msg) => (
              <li
                key={msg.id}
                className={`page-chat__message page-chat__message--${msg.role}`}
                data-role={msg.role}
              >
                <span className="page-chat__message-role">{msg.role}</span>
                <div className="page-chat__message-content">
                  {msg.role === "assistant" ? (
                    <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                  ) : (
                    <p>{msg.content}</p>
                  )}
                </div>
              </li>
            ))}
            {streamingContent !== null && (
              <li
                className="page-chat__message page-chat__message--assistant"
                data-role="assistant"
                aria-live="polite"
              >
                <span className="page-chat__message-role">assistant</span>
                <div className="page-chat__message-content">
                  {streamingContent ? (
                    <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
                  ) : (
                    <p className="page-chat__text page-chat__muted">Thinking…</p>
                  )}
                </div>
              </li>
            )}
          </ul>
        )}
      </section>
      <section className="page-chat__input" aria-label="Send a message">
        <div className="page-chat__input-box">
          <textarea
            className="page-chat__follow-up-input"
            value={input}
            onChange={(e) => {
              setInput(e.target.value)
              setSendError(null)
            }}
            placeholder="Type a message…"
            rows={2}
            disabled={sending}
            aria-label="Message"
          />
          <button
            type="button"
            className="page-chat__follow-up-btn"
            onClick={handleSend}
            disabled={sending || !input.trim()}
          >
            {sending ? "Sending…" : "Send"}
          </button>
        </div>
        {sendError && (
          <p className="page-chat__text page-chat__error" role="alert">
            {sendError}
          </p>
        )}
      </section>
    </div>
  )
}
