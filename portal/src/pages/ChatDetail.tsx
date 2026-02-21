import { useEffect, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { Chat } from "../lib/types"
import { getErrorMessage } from "../lib/errorMessage"
import { getChatConversation, createChatRun, getChats } from "../lib/api"
import { useAuth } from "../contexts/AuthContext"
import { useFetch } from "../hooks/useFetch"
import { UserAvatar, AgentAvatar } from "../components/UserAvatar"

const POLL_INTERVAL_MS = 2000
const TERMINAL_STATUSES = ["SUCCEEDED", "FAILED"]

interface ChatDetailProps {
  chat: Chat
  workspaceId: string
  onRefetch?: () => void
}

export function ChatDetail({ chat, workspaceId, onRefetch }: ChatDetailProps) {
  const { token, user } = useAuth()
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const {
    data: session,
    loading: sessionLoading,
    error: sessionError,
    refetch: refetchSession,
  } = useFetch(
    () => getChatConversation(workspaceId, chat.id, token!),
    [workspaceId, chat.id, token],
    {
      enabled: !!(token && workspaceId && chat.id),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load session"),
    }
  )

  const [followUpInput, setFollowUpInput] = useState("")
  const [followUpLoading, setFollowUpLoading] = useState(false)
  const [followUpError, setFollowUpError] = useState<string | null>(null)
  const [expandedToolIndices, setExpandedToolIndices] = useState<Set<number>>(new Set())

  function toggleToolExpand(index: number) {
    setExpandedToolIndices((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
        pollIntervalRef.current = null
      }
    }
  }, [])

  async function handleFollowUpSubmit() {
    const input = followUpInput.trim()
    if (!input || !token || followUpLoading) return
    setFollowUpError(null)
    setFollowUpLoading(true)
    try {
      await createChatRun(workspaceId, chat.id, { input }, token)
      setFollowUpInput("")
      pollIntervalRef.current = setInterval(async () => {
        if (!token) return
        try {
          const list = await getChats(workspaceId, token)
          const updated = list.find((c) => c.id === chat.id)
          if (updated && TERMINAL_STATUSES.includes(updated.status)) {
            if (pollIntervalRef.current) {
              clearInterval(pollIntervalRef.current)
              pollIntervalRef.current = null
            }
            setFollowUpLoading(false)
            onRefetch?.()
            refetchSession()
          }
        } catch {
          // ignore poll errors; keep polling
        }
      }, POLL_INTERVAL_MS)
    } catch (err) {
      setFollowUpError(getErrorMessage(err, "Failed to start run"))
      setFollowUpLoading(false)
    }
  }

  return (
    <div className="page-chat">
      <section className="page-chat__history">
        {sessionLoading && (
          <p className="page-chat__text page-chat__muted">Loading conversation…</p>
        )}
        {sessionError && (
          <p className="page-chat__text page-chat__muted">Error: {sessionError}</p>
        )}
        {session && !sessionLoading && !sessionError && (
          <>
            {session.messages.length === 0 && (
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
                          <span className="page-chat__tool-toggle-label">
                            Tool result
                          </span>
                          <span
                            className="page-chat__tool-chevron"
                            aria-hidden
                          >
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
                              <Markdown remarkPlugins={[remarkGfm]}>
                                {msg.content}
                              </Markdown>
                            </div>
                          ) : null}
                        </div>
                      </>
                    ) : (
                      <>
                        {msg.content ? (
                          <div className="page-chat__msg-content page-chat__markdown">
                            <Markdown remarkPlugins={[remarkGfm]}>
                              {msg.content}
                            </Markdown>
                          </div>
                        ) : null}
                        {msg.tool_calls && msg.tool_calls.length > 0 && (
                          <ul className="page-chat__msg-toolcalls">
                            {msg.tool_calls.map((tc) => (
                              <li key={tc.id}>
                                <strong>{tc.name}</strong>
                                {tc.arguments ? (
                                  <pre className="page-chat__msg-args">
                                    {tc.arguments}
                                  </pre>
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
          </>
        )}
      </section>

      <section className="page-chat__input">
        <div className="page-chat__input-box">
          <textarea
            className="page-chat__follow-up-input"
            value={followUpInput}
            onChange={(e) => setFollowUpInput(e.target.value)}
            placeholder="Ask a follow-up question…"
            rows={2}
            disabled={followUpLoading}
            aria-label="Chat input"
          />
          <button
            type="button"
            className="page-chat__follow-up-btn"
            onClick={handleFollowUpSubmit}
            disabled={followUpLoading || !followUpInput.trim()}
          >
            {followUpLoading ? "Running…" : "Send"}
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
