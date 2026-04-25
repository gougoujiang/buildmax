import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type { Agent, Issue } from "../lib/types"

interface IssueModalProps {
  open: boolean
  mode: "create" | "edit"
  issue?: Issue | null
  agents: Agent[]
  userId?: string
  loading: boolean
  error: string | null
  onClose: () => void
  onSubmit: (values: {
    title: string
    description: string
    status: Issue["status"]
    assignee_kind: "person" | "agent" | ""
    assignee_id: string
  }) => void
}

export function IssueModal({
  open,
  mode,
  issue,
  agents,
  userId,
  loading,
  error,
  onClose,
  onSubmit,
}: IssueModalProps) {
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [status, setStatus] = useState<Issue["status"]>("todo")
  const [assigneeValue, setAssigneeValue] = useState("")

  useEffect(() => {
    if (!open) return
    if (mode === "edit" && issue) {
      setTitle(issue.title)
      setDescription(issue.description)
      setStatus(issue.status)
      setAssigneeValue(
        issue.assigneeKind === "person"
          ? `person:${issue.assigneeId ?? ""}`
          : issue.assigneeKind === "agent"
            ? `agent:${issue.assigneeId ?? ""}`
            : "",
      )
      return
    }
    setTitle("")
    setDescription("")
    setStatus("todo")
    setAssigneeValue("")
  }, [open, mode, issue])

  const titleText = mode === "create" ? "New Issue" : "Issue Details"
  const submitText = mode === "create" ? "Create issue" : "Save"

  return (
    <BaseModal
      open={open}
      title={titleText}
      titleId="issue-modal-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <div className="issues-page__form">
          <label className="issues-page__field">
            <span className="issues-page__field-label">Title</span>
            <input className="issues-page__input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="What needs to be done?" />
          </label>
          <label className="issues-page__field">
            <span className="issues-page__field-label">Description</span>
            <textarea className="issues-page__textarea" rows={6} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Add more context" />
          </label>
          <label className="issues-page__field">
            <span className="issues-page__field-label">Status</span>
            <select className="issues-page__select" value={status} onChange={(e) => setStatus(e.target.value as Issue["status"])}>
              <option value="todo">todo</option>
              <option value="in_progress">in_progress</option>
              <option value="done">done</option>
            </select>
          </label>
          <label className="issues-page__field">
            <span className="issues-page__field-label">Assignee</span>
            <select className="issues-page__select" value={assigneeValue} onChange={(e) => setAssigneeValue(e.target.value)}>
              <option value="">Unassigned</option>
              {userId ? <option value={`person:${userId}`}>Me</option> : null}
              {agents.map((agent) => (
                <option key={agent.id} value={`agent:${agent.id}`}>{agent.name}</option>
              ))}
            </select>
          </label>
          {mode === "edit" && issue ? (
            <div className="issues-page__meta-row">
              <div className="page-activity__meta">Created: {new Date(issue.createdAt * 1000).toLocaleString()}</div>
              <div className="page-activity__meta">Updated: {new Date(issue.updatedAt * 1000).toLocaleString()}</div>
            </div>
          ) : null}
          {error ? (
            <p className="modal__error" role="alert">
              {error}
            </p>
          ) : null}
          <div className="modal__actions">
            <button type="button" className="modal__btn modal__btn--secondary" onClick={onClose} disabled={loading}>
              Cancel
            </button>
            <button
              type="button"
              className="modal__btn modal__btn--secondary"
              disabled={loading || !title.trim()}
              onClick={() => {
                const [kind, id] = assigneeValue ? assigneeValue.split(":") : ["", ""]
                onSubmit({
                  title: title.trim(),
                  description,
                  status,
                  assignee_kind: (kind as "person" | "agent" | "") || "",
                  assignee_id: id || "",
                })
              }}
            >
              {loading ? `${submitText}…` : submitText}
            </button>
          </div>
        </div>
      </div>
    </BaseModal>
  )
}
