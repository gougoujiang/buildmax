import { useEffect, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { Task } from "../lib/types"
import { getTaskConversation, createTaskRun, getTasks } from "../lib/api"
import { useAuth } from "../contexts/AuthContext"
import { useFetch } from "../hooks/useFetch"

const POLL_INTERVAL_MS = 2000
const TERMINAL_STATUSES = ["SUCCEEDED", "FAILED"]

interface TaskDetailProps {
  task: Task
  workspaceId: string
  onRefetch?: () => void
}

export function TaskDetail({ task, workspaceId, onRefetch }: TaskDetailProps) {
  const { token } = useAuth()
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const {
    data: session,
    loading: sessionLoading,
    error: sessionError,
    refetch: refetchSession,
  } = useFetch(
    () => getTaskConversation(workspaceId, task.id, token!),
    [workspaceId, task.id, token],
    {
      enabled: !!(token && workspaceId && task.id),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load session"),
    }
  )

  const [followUpInput, setFollowUpInput] = useState("")
  const [followUpLoading, setFollowUpLoading] = useState(false)
  const [followUpError, setFollowUpError] = useState<string | null>(null)

  // Clear poll on unmount
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
      await createTaskRun(workspaceId, task.id, { input }, token)
      setFollowUpInput("")
      // Poll until task status is SUCCEEDED or FAILED
      pollIntervalRef.current = setInterval(async () => {
        if (!token) return
        try {
          const list = await getTasks(workspaceId, token)
          const updated = list.find((t) => t.id === task.id)
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
      setFollowUpError(err instanceof Error ? err.message : "Failed to start run")
      setFollowUpLoading(false)
    }
  }

  return (
    <div className="page-task">
      <header className="page-task__header">
        <h1 className="page-task__title">{task.title?.trim() || "Chat"}</h1>
        <button
          type="button"
          className="page-task__restore-btn"
          disabled
          title="Restore is not yet available"
        >
          Restore
        </button>
      </header>

      {/* Follow-up: rerun with new input */}
      <section className="page-task__section page-task__follow-up">
        <h2 className="page-task__section-heading">Follow-up</h2>
        <p className="page-task__text page-task__muted">
          Add more context or a new question to run the task again. The agent will use the previous conversation.
        </p>
        <div className="page-task__follow-up-row">
          <textarea
            className="page-task__follow-up-input"
            value={followUpInput}
            onChange={(e) => setFollowUpInput(e.target.value)}
            placeholder="e.g. Now focus on Q3 only"
            rows={2}
            disabled={followUpLoading}
            aria-label="Follow-up input"
          />
          <button
            type="button"
            className="page-task__follow-up-btn"
            onClick={handleFollowUpSubmit}
            disabled={followUpLoading || !followUpInput.trim()}
          >
            {followUpLoading ? "Running…" : "Run follow-up"}
          </button>
        </div>
        {followUpError && (
          <p className="page-task__text page-task__error" role="alert">
            {followUpError}
          </p>
        )}
      </section>

      {/* Result — rendered as markdown */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Result</h2>
        <div className="page-task__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{task.summary}</Markdown>
        </div>
      </section>

      {/* Agent conversation — how the agent worked for this task */}
      {(session !== null || sessionLoading || sessionError) && (
        <section className="page-task__section">
          <h2 className="page-task__section-heading">Agent session</h2>
          {sessionLoading && (
            <p className="page-task__text page-task__muted">Loading conversation…</p>
          )}
          {sessionError && (
            <p className="page-task__text page-task__muted">Error: {sessionError}</p>
          )}
          {session && !sessionLoading && (
            <div className="page-task__session">
              {session.messages.length === 0 && (
                <p className="page-task__text page-task__muted">No messages in this session.</p>
              )}
              {session.messages.map((msg, i) => (
                <div key={i} className={`page-task__session-msg page-task__session-msg--${msg.role}`}>
                  <span className="page-task__session-role">{msg.role}</span>
                  {msg.content ? (
                    <div className="page-task__session-content">
                      <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                    </div>
                  ) : null}
                  {msg.tool_calls && msg.tool_calls.length > 0 && (
                    <ul className="page-task__session-toolcalls">
                      {msg.tool_calls.map((tc) => (
                        <li key={tc.id}>
                          <strong>{tc.name}</strong>
                          {tc.arguments ? (
                            <pre className="page-task__session-args">{tc.arguments}</pre>
                          ) : null}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {/* What changed — not yet available from backend */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">What changed</h2>
        <p className="page-task__text page-task__muted">Not yet available.</p>
      </section>

      {/* Evidence / Data used — not yet available from backend */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Evidence / Data used</h2>
        <p className="page-task__text page-task__muted">Not yet available.</p>
      </section>

      {/* Meta */}
      <section className="page-task__section">
        <h2 className="page-task__section-heading">Details</h2>
        <p className="page-task__meta">
          Status: <strong>{task.status}</strong> &middot; {task.timeLabel}
        </p>
      </section>
    </div>
  )
}
