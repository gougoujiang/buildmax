import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { AgentAvatar, UserAvatar } from "../../../components/UserAvatar"
import type { LoginUser } from "../../../lib/api"
import type { ApiSession } from "../../../lib/api"

interface ChatDetailViewProps {
  historyRef: React.RefObject<HTMLElement | null>
  session: ApiSession | null
  sessionLoading: boolean
  sessionError: string | null
  followUpInput: string
  setFollowUpInput: (value: string) => void
  followUpLoading: boolean
  followUpError: string | null
  streamingContent: string
  lastSentMessage: string | null
  user: LoginUser | null
  initialInput?: string
  showInitialInput: boolean
  expandedToolIndices: Set<number>
  toggleToolExpand: (index: number) => void
  onSubmitFollowUp: () => void
}

export function ChatDetailView({
  historyRef,
  session,
  sessionLoading,
  sessionError,
  followUpInput,
  setFollowUpInput,
  followUpLoading,
  followUpError,
  streamingContent,
  lastSentMessage,
  user,
  initialInput,
  showInitialInput,
  expandedToolIndices,
  toggleToolExpand,
  onSubmitFollowUp,
}: ChatDetailViewProps) {
  return (
    <div className="page-chat">
      <section ref={historyRef} className="page-chat__history" aria-label="Chat history">
        {sessionLoading && (
          <p className="page-chat__text page-chat__muted">Loading conversation…</p>
        )}
        {sessionError && (
          <p className="page-chat__text page-chat__muted">Error: {sessionError}</p>
        )}
        {showInitialInput && initialInput && (
          <div
            className="page-chat__msg-row page-chat__msg-row--user"
            role="article"
            aria-label="You"
          >
            <span className="page-chat__msg-icon" aria-hidden>
              {user ? <UserAvatar user={user} size="sm" /> : <AgentAvatar size="sm" />}
            </span>
            <div className="page-chat__msg page-chat__msg--user">
              <div className="page-chat__msg-content page-chat__markdown">
                <Markdown remarkPlugins={[remarkGfm]}>{initialInput}</Markdown>
              </div>
            </div>
          </div>
        )}
        {session && !sessionLoading && !sessionError && (
          <>
            {session.messages.length === 0 && !initialInput && (
              <p className="page-chat__text page-chat__muted">
                No messages yet. Use the input below to start.
              </p>
            )}
            {session.messages.map((msg, i) => {
              const isUser = msg.role === "user"
              const isTool = msg.role === "tool"
              const isToolExpanded = expandedToolIndices.has(i)

              return (
                <div
                  key={i}
                  className={`page-chat__msg-row page-chat__msg-row--${msg.role}`}
                  role="article"
                  aria-label={isUser ? "You" : msg.role}
                >
                  {!isTool && (
                    <span className="page-chat__msg-icon" aria-hidden>
                      {isUser && user ? (
                        <UserAvatar user={user} size="sm" />
                      ) : (
                        <AgentAvatar size="sm" />
                      )}
                    </span>
                  )}
                  <div
                    className={`page-chat__msg page-chat__msg--${msg.role}${
                      isTool && !isToolExpanded ? " page-chat__msg--tool-collapsed" : ""
                    }`}
                  >
                    {isTool ? (
                      <>
                        <button
                          type="button"
                          className="page-chat__tool-toggle"
                          onClick={() => toggleToolExpand(i)}
                          aria-expanded={isToolExpanded}
                          aria-controls={`tool-content-${i}`}
                          id={`tool-toggle-${i}`}
                        >
                          <span className="page-chat__tool-toggle-label">Tool result</span>
                          <span className="page-chat__tool-chevron" aria-hidden>
                            {isToolExpanded ? "▲" : "▼"}
                          </span>
                        </button>
                        <div
                          id={`tool-content-${i}`}
                          className="page-chat__tool-content"
                          hidden={!isToolExpanded}
                          role="region"
                          aria-labelledby={`tool-toggle-${i}`}
                        >
                          {msg.content ? (
                            <div className="page-chat__msg-content page-chat__markdown">
                              <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                            </div>
                          ) : null}
                        </div>
                      </>
                    ) : (
                      <>
                        {msg.content ? (
                          <div className="page-chat__msg-content page-chat__markdown">
                            <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                          </div>
                        ) : null}
                        {msg.tool_calls && msg.tool_calls.length > 0 && (
                          <ul className="page-chat__msg-toolcalls">
                            {msg.tool_calls.map((tc) => (
                              <li key={tc.id}>
                                <strong>{tc.name}</strong>
                                {tc.arguments ? (
                                  <pre className="page-chat__msg-args">{tc.arguments}</pre>
                                ) : null}
                              </li>
                            ))}
                          </ul>
                        )}
                      </>
                    )}
                  </div>
                </div>
              )
            })}
            {lastSentMessage ? (
              <div className="page-chat__msg-row page-chat__msg-row--user" role="article" aria-label="You">
                <span className="page-chat__msg-icon" aria-hidden>
                  {user ? <UserAvatar user={user} size="sm" /> : <AgentAvatar size="sm" />}
                </span>
                <div className="page-chat__msg page-chat__msg--user">
                  <div className="page-chat__msg-content page-chat__markdown">
                    <Markdown remarkPlugins={[remarkGfm]}>{lastSentMessage}</Markdown>
                  </div>
                </div>
              </div>
            ) : null}
            {streamingContent ? (
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
                    <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
                  </div>
                </div>
              </div>
            ) : null}
          </>
        )}
      </section>

      <section className="page-chat__input">
        <div className="page-chat__input-box">
          <textarea
            className="page-chat__follow-up-input"
            value={followUpInput}
            onChange={(e) => setFollowUpInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault()
                if (followUpInput.trim() && !followUpLoading) onSubmitFollowUp()
              }
            }}
            placeholder="Ask a follow-up… (Enter to send, Shift+Enter for new line)"
            rows={2}
            disabled={followUpLoading}
            aria-label="Chat input"
          />
          <button
            type="button"
            className="page-chat__follow-up-btn"
            onClick={onSubmitFollowUp}
            disabled={followUpLoading || !followUpInput.trim()}
          >
            {followUpLoading ? "Sending…" : "Send"}
          </button>
        </div>
        {followUpError && (
          <p className="page-chat__text page-chat__error" role="alert">
            {followUpError}
          </p>
        )}
      </section>
    </div>
  )
}
