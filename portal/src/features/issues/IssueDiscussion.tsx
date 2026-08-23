import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import type { ApiIssueComment, ApiTeamMember } from "../../lib/api/types"
import { createIssueComment, deleteIssueComment, getIssueComments, updateIssueComment } from "./comments"
import { getErrorMessage } from "../../lib/errorMessage"

/** Matches CommentBodyLimit in internal/service/issue. */
const BODY_LIMIT = 16 * 1024
/** Where the counter appears, so a long comment warns before the server refuses it. */
const COUNTER_THRESHOLD = 15 * 1024

/**
 * How often the thread reloads. There is no push channel for comments yet, so
 * two people commenting at once see each other within this window.
 */
const POLL_INTERVAL_MS = 20_000

interface IssueDiscussionProps {
  teamId: string | null
  issueId: string | null
  token: string | null
  userId: string | null
  /** True when the caller may delete comments they did not write. */
  canModerate: boolean
  members: ApiTeamMember[]
  agentNames: Record<string, string>
  onOpenTrace?: (taskRunId: string) => void
  /**
   * Reports the current thread whenever it changes. This component owns the
   * fetch — the page reads the result rather than requesting it a second time.
   * The callback must be stable, or it will re-fire on every render.
   */
  onCommentsChanged?: (comments: ApiIssueComment[]) => void
}

function formatTimestamp(rfc3339: string): string {
  return new Date(rfc3339).toLocaleString()
}

export function IssueDiscussion({
  teamId,
  issueId,
  token,
  userId,
  canModerate,
  members,
  agentNames,
  onOpenTrace,
  onCommentsChanged,
}: IssueDiscussionProps) {
  const [comments, setComments] = useState<ApiIssueComment[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [draft, setDraft] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState("")
  // Tracks the latest load so a slow response cannot overwrite a newer one.
  const loadSeq = useRef(0)

  const load = useCallback(
    async (showSpinner: boolean) => {
      if (!teamId || !issueId || !token) return
      const seq = ++loadSeq.current
      if (showSpinner) setLoading(true)
      try {
        const res = await getIssueComments(teamId, issueId, token, { limit: 200 })
        if (seq === loadSeq.current) {
          setComments(res.comments ?? [])
          setError(null)
        }
      } catch (err) {
        if (seq === loadSeq.current) setError(getErrorMessage(err, "Failed to load comments"))
      } finally {
        if (seq === loadSeq.current && showSpinner) setLoading(false)
      }
    },
    [teamId, issueId, token],
  )

  useEffect(() => {
    void load(true)
  }, [load])

  useEffect(() => {
    if (!teamId || !issueId || !token) return
    const timer = window.setInterval(() => {
      void load(false)
    }, POLL_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [load, teamId, issueId, token])

  useEffect(() => {
    onCommentsChanged?.(comments)
  }, [comments, onCommentsChanged])

  const memberNames = useMemo(() => {
    const out: Record<string, string> = {}
    for (const member of members) {
      out[member.user_id] = member.user_name || member.user_email || `Member ${member.user_id.slice(0, 8)}`
    }
    return out
  }, [members])

  function authorLabel(comment: ApiIssueComment): string {
    if (comment.author_kind === "agent") return agentNames[comment.author_id] || "Agent"
    if (comment.author_kind === "system") return "BuildMax"
    if (comment.author_id === userId) return "Me"
    return memberNames[comment.author_id] || `Member ${comment.author_id.slice(0, 8)}`
  }

  function canEdit(comment: ApiIssueComment): boolean {
    return comment.author_kind === "user" && comment.author_id === userId
  }

  function canDelete(comment: ApiIssueComment): boolean {
    return canEdit(comment) || canModerate
  }

  async function handleSubmit() {
    if (!teamId || !issueId || !token) return
    const body = draft.trim()
    if (!body || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      const created = await createIssueComment(teamId, issueId, body, token)
      setComments((prev) => [...prev, created])
      setDraft("")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to post comment"))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleSaveEdit(commentId: string) {
    if (!teamId || !issueId || !token) return
    const body = editDraft.trim()
    if (!body) return
    try {
      const updated = await updateIssueComment(teamId, issueId, commentId, body, token)
      setComments((prev) => prev.map((c) => (c.id === commentId ? updated : c)))
      setEditingId(null)
      setEditDraft("")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to save comment"))
    }
  }

  async function handleDelete(commentId: string) {
    if (!teamId || !issueId || !token) return
    // Deletion is permanent — the row is removed, not tombstoned.
    if (!window.confirm("Delete this comment? This cannot be undone.")) return
    try {
      await deleteIssueComment(teamId, issueId, commentId, token)
      setComments((prev) => prev.filter((c) => c.id !== commentId))
    } catch (err) {
      setError(getErrorMessage(err, "Failed to delete comment"))
    }
  }

  function handleComposerKeyDown(event: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault()
      void handleSubmit()
    }
  }

  return (
    <div className="issue-discussion">
      {error ? (
        <p className="modal__error" role="alert">
          {error}
        </p>
      ) : null}
      {loading ? (
        <p className="page-activity__empty">Loading…</p>
      ) : comments.length === 0 ? (
        <p className="page-activity__empty">No comments yet.</p>
      ) : (
        <ol className="issue-discussion__list">
          {comments.map((comment) => (
            <li key={comment.id} className="issue-discussion__item">
              <div className="issue-discussion__head">
                <span className={`issue-discussion__author issue-discussion__author--${comment.author_kind}`}>
                  {authorLabel(comment)}
                </span>
                <span className="page-activity__meta">
                  {formatTimestamp(comment.created_at)}
                  {comment.edited_at ? " · edited" : ""}
                </span>
              </div>
              {editingId === comment.id ? (
                <div className="issue-discussion__composer">
                  <textarea
                    className="issues-page__input"
                    value={editDraft}
                    maxLength={BODY_LIMIT}
                    rows={4}
                    onChange={(e) => setEditDraft(e.target.value)}
                  />
                  <div className="issue-discussion__actions">
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      onClick={() => void handleSaveEdit(comment.id)}
                      disabled={editDraft.trim() === ""}
                    >
                      Save
                    </button>
                    <button type="button" className="page-activity__action-btn" onClick={() => setEditingId(null)}>
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <p className="issue-discussion__body">{comment.body}</p>
              )}
              <div className="issue-discussion__actions">
                {comment.source_task_run_id && onOpenTrace ? (
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => onOpenTrace(comment.source_task_run_id!)}
                  >
                    Run details
                  </button>
                ) : null}
                {canEdit(comment) && editingId !== comment.id ? (
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => {
                      setEditingId(comment.id)
                      setEditDraft(comment.body)
                    }}
                  >
                    Edit
                  </button>
                ) : null}
                {canDelete(comment) ? (
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => void handleDelete(comment.id)}
                  >
                    Delete
                  </button>
                ) : null}
              </div>
            </li>
          ))}
        </ol>
      )}
      <div className="issue-discussion__composer">
        <textarea
          className="issues-page__input"
          value={draft}
          maxLength={BODY_LIMIT}
          rows={3}
          placeholder="Write a comment. Cmd/Ctrl+Enter to post."
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleComposerKeyDown}
        />
        <div className="issue-discussion__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => void handleSubmit()}
            disabled={submitting || draft.trim() === ""}
          >
            {submitting ? "Posting…" : "Comment"}
          </button>
          {draft.length > COUNTER_THRESHOLD ? (
            <span className="page-activity__meta">
              {draft.length} / {BODY_LIMIT}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  )
}
