import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { ApiConversationMessage } from "../../../lib/api"

interface ConversationDetailViewProps {
  historyRef: React.RefObject<HTMLElement | null>
  messages: ApiConversationMessage[]
  messagesLoading: boolean
  messagesError: string | null
  input: string
  setInput: (value: string) => void
  sending: boolean
  sendError: string | null
  streamingContent: string | null
  onSend: () => void
}

export function ConversationDetailView({
  historyRef,
  messages,
  messagesLoading,
  messagesError,
  input,
  setInput,
  sending,
  sendError,
  streamingContent,
  onSend,
}: ConversationDetailViewProps) {
  return (
    <div className="page-chat">
      <section ref={historyRef} className="page-chat__history" aria-label="Conversation history">
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
            onChange={(e) => setInput(e.target.value)}
            placeholder="Type a message…"
            rows={2}
            disabled={sending}
            aria-label="Message"
          />
          <button
            type="button"
            className="page-chat__follow-up-btn"
            onClick={onSend}
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
