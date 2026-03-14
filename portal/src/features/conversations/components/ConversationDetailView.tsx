import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { AgentAvatar, UserAvatar } from "../../../components/UserAvatar"
import type { ApiConversationMessage, LoginUser } from "../../../lib/api"

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
  user: LoginUser | null
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
  user,
  onSend,
}: ConversationDetailViewProps) {
  return (
    <div className="page-chat">
      <section ref={historyRef} className="page-chat__history" aria-label="Conversation history">
        {messagesLoading && (
          <p className="page-chat__text page-chat__muted">Loading conversation…</p>
        )}
        {messagesError && (
          <p className="page-chat__text page-chat__error" role="alert">
            {messagesError}
          </p>
        )}
        {!messagesLoading && !messagesError && (
          <>
            {messages.length === 0 && (
              <p className="page-chat__text page-chat__muted">
                No messages yet. Use the input below to start.
              </p>
            )}
            {messages.map((msg) => {
              const isUser = msg.role === "user"
              return (
                <div
                  key={msg.id}
                  className={`page-chat__msg-row page-chat__msg-row--${msg.role}`}
                  role="article"
                  aria-label={isUser ? "You" : msg.role}
                >
                  <span className="page-chat__msg-icon" aria-hidden>
                    {isUser && user ? (
                      <UserAvatar user={user} size="sm" />
                    ) : (
                      <AgentAvatar size="sm" />
                    )}
                  </span>
                  <div className={`page-chat__msg page-chat__msg--${msg.role}`}>
                    {msg.content ? (
                      <div className="page-chat__msg-content page-chat__markdown">
                        <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                      </div>
                    ) : null}
                  </div>
                </div>
              )
            })}
            {streamingContent !== null ? (
              <div
                className="page-chat__msg-row page-chat__msg-row--assistant"
                role="article"
                aria-label="Assistant (streaming)"
              >
                <span className="page-chat__msg-icon" aria-hidden>
                  <AgentAvatar size="sm" />
                </span>
                <div className="page-chat__msg page-chat__msg--assistant">
                  <div className="page-chat__msg-content page-chat__markdown">
                    {streamingContent ? (
                      <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
                    ) : (
                      <p className="page-chat__text page-chat__muted">Thinking…</p>
                    )}
                  </div>
                </div>
              </div>
            ) : null}
          </>
        )}
      </section>

      <section className="page-chat__input" aria-label="Send a message">
        <div className="page-chat__input-box">
          <textarea
            className="page-chat__follow-up-input"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                if (input.trim() && !sending) onSend()
              }
            }}
            placeholder="Type a message… (Enter to send, Shift+Enter for new line)"
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
