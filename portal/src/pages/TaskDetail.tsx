import { useEffect, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { Task } from "../lib/types"
import type { ApiSession } from "../lib/api"
import { getTaskConversation } from "../lib/api"
import { useAuth } from "../contexts/AuthContext"

interface TaskDetailProps {
  task: Task
  workspaceId: string
}

export function TaskDetail({ task, workspaceId }: TaskDetailProps) {
  const { token } = useAuth()
  const [session, setSession] = useState<ApiSession | null>(null)
  const [sessionLoading, setSessionLoading] = useState(false)
  const [sessionError, setSessionError] = useState<string | null>(null)

  useEffect(() => {
    if (!token || !workspaceId || !task.id) {
      setSession(null)
      setSessionError(null)
      return
    }
    let cancelled = false
    setSessionLoading(true)
    setSessionError(null)
    getTaskConversation(workspaceId, task.id, token)
      .then((data) => {
        if (!cancelled) {
          setSession(data)
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setSessionError(err instanceof Error ? err.message : "Failed to load session")
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSessionLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [workspaceId, task.id, token])

  return (
    <div className="page-task">
      <header className="page-task__header">
        <h1 className="page-task__title">Task: {task.title}</h1>
        <button
          type="button"
          className="page-task__restore-btn"
          disabled
          title="Restore is not yet available"
        >
          Restore
        </button>
      </header>

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
