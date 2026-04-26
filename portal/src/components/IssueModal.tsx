import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type { ApiTeamMember } from "../lib/api/types"
import type { Agent, Issue, Workflow } from "../lib/types"

interface IssueModalProps {
  open: boolean
  mode: "create" | "edit"
  issue?: Issue | null
  agents: Agent[]
  workflows: Workflow[]
  members: ApiTeamMember[]
  userId?: string
  loading: boolean
  runningWorkflow?: boolean
  allowWorkflowAssignment?: boolean
  error: string | null
  onClose: () => void
  onSubmit: (values: {
    title: string
    description: string
    status: Issue["status"]
    assignee_kind: "person" | "agent" | "workflow" | ""
    assignee_id: string
  }) => void
  onRunWorkflow?: () => void
}

export function IssueModal({
  open,
  mode,
  issue,
  agents,
  workflows,
  members,
  userId,
  loading,
  runningWorkflow = false,
  allowWorkflowAssignment = true,
  error,
  onClose,
  onSubmit,
  onRunWorkflow,
}: IssueModalProps) {
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [status, setStatus] = useState<Issue["status"]>("todo")
  const [assigneeValue, setAssigneeValue] = useState("")
  const selectedWorkflowId = assigneeValue.startsWith("workflow:")
    ? assigneeValue.slice("workflow:".length)
    : issue?.assigneeKind === "workflow"
      ? issue.assigneeId ?? ""
      : ""
  const selectableWorkflows = workflows.filter(
    (workflow) => workflow.status === "published" || workflow.id === selectedWorkflowId,
  )

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
              : issue.assigneeKind === "workflow"
                ? `workflow:${issue.assigneeId ?? ""}`
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

  function memberLabel(member: ApiTeamMember): string {
    if (member.user_id === userId) return "Me"
    if (member.user_name && member.user_name.trim() !== "") return member.user_name
    if (member.user_email && member.user_email.trim() !== "") return member.user_email
    return `Member ${member.user_id.slice(0, 8)}`
  }

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
              {members.map((member) => (
                <option key={member.user_id} value={`person:${member.user_id}`}>
                  {memberLabel(member)}
                </option>
              ))}
              {agents.map((agent) => (
                    <option key={agent.id} value={`agent:${agent.id}`}>{agent.name}</option>
              ))}
              {allowWorkflowAssignment
                ? selectableWorkflows.map((workflow) => (
                    <option key={workflow.id} value={`workflow:${workflow.id}`}>
                      {workflow.name}{workflow.status !== "published" ? ` (${workflow.status})` : ""}
                    </option>
                  ))
                : null}
            </select>
            <span className="issues-page__field-label">
              {allowWorkflowAssignment
                ? "Only `published` workflows are available for new assignment."
                : "You can assign a person or agent here. Workflow assignment is limited to team owners and admins."}
            </span>
          </label>
          {mode === "edit" && issue?.assigneeKind === "workflow" ? (
            <div className="workflow-page__inline-actions">
              <button
                type="button"
                className="page-activity__action-btn"
                disabled={runningWorkflow}
                onClick={onRunWorkflow}
              >
                {runningWorkflow ? "Running…" : "Run Workflow"}
              </button>
            </div>
          ) : null}
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
                  assignee_kind: (kind as "person" | "agent" | "workflow" | "") || "",
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
