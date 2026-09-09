import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, Issue } from "../../lib/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import { apiAgentToAgent, apiIssueToIssue, apiWorkflowToWorkflow } from "../../lib/api/mappers"
import { createIssue, getIssues, updateIssue } from "../../features/issues"
import { getAgents } from "../../features/agents"
import { getSpaceMembers } from "../../features/spaces/api"
import { getWorkflows } from "../../features/workflows"
import { IssueModal } from "../../components/IssueModal"
import { useSpace } from "../../contexts/SpaceContext"
import type { ApiSpaceMember } from "../../lib/api/types"
import type { Workflow } from "../../lib/types"

const PAGE_SIZE = 10

interface IssuesProps {
  token: string | null
  spaceId: string
  userId?: string
}

export function Issues({ token, spaceId, userId }: IssuesProps) {
  const { currentUserRole } = useSpace()
  const [issues, setIssues] = useState<Issue[]>([])
  const [total, setTotal] = useState(0)
  const [agents, setAgents] = useState<Agent[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [members, setMembers] = useState<ApiSpaceMember[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  // An entry appears here once a parent's children have been fetched; undefined
  // while the request is in flight.
  const [children, setChildren] = useState<Record<string, Issue[]>>({})
  const canAssignWorkflow = currentUserRole === "owner" || currentUserRole === "admin"

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const fetchIssues = useCallback(() => {
    if (!token || !spaceId) {
      setIssues([])
      setAgents([])
      setWorkflows([])
      setMembers([])
      setTotal(0)
      setLoading(false)
      return Promise.resolve()
    }
    setLoading(true)
    setError(null)
    return Promise.all([
      // The board shows top-level issues; sub-issues appear under the parent
      // they were split out of, not as siblings in the same list.
      getIssues(spaceId, token, { limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE, parentId: "none" }),
      getAgents(spaceId, token),
      getSpaceMembers(spaceId, token),
      getWorkflows(spaceId, token),
    ])
      .then(([issueRes, agentRes, memberRes, workflowRes]) => {
        setIssues(issueRes.issues.map(apiIssueToIssue))
        setTotal(issueRes.total)
        setAgents(agentRes.map(apiAgentToAgent))
        setMembers(memberRes)
        setWorkflows(workflowRes.workflows.map(apiWorkflowToWorkflow))
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to load issues")))
      .finally(() => setLoading(false))
  }, [page, token, spaceId])

  useEffect(() => {
    void fetchIssues()
  }, [fetchIssues])

  useEffect(() => {
    setPage(1)
  }, [spaceId])

  // A reload invalidates every cached breakdown: statuses may have moved, and a
  // stale child list is worse than a second fetch.
  useEffect(() => {
    setExpanded({})
    setChildren({})
  }, [page, spaceId])

  function toggleChildren(issueId: string) {
    const nowOpen = !expanded[issueId]
    setExpanded((prev) => ({ ...prev, [issueId]: nowOpen }))
    if (!nowOpen || !token || !spaceId || children[issueId] !== undefined) return
    getIssues(spaceId, token, { limit: 100, parentId: issueId })
      .then((res) => setChildren((prev) => ({ ...prev, [issueId]: res.issues.map(apiIssueToIssue) })))
      .catch((err) => setError(getErrorMessage(err, "Failed to load sub-issues")))
  }

  // What is being done needs both halves at a glance: who is accountable and
  // what will execute, since an Issue can have one, the other, both, or
  // neither.
  function ownerLabel(issue: Issue): string | null {
    if (!issue.ownerId) return null
    if (issue.ownerId === userId) return "Me"
    const member = members.find((item) => item.user_id === issue.ownerId)
    if (member?.user_name) return member.user_name
    if (member?.user_email) return member.user_email
    if (member) return `Member ${member.user_id.slice(0, 8)}`
    return "Member"
  }

  function executorLabel(issue: Issue): string | null {
    if (issue.executorKind === "agent") {
      return agents.find((agent) => agent.id === issue.executorId)?.name || "Agent"
    }
    if (issue.executorKind === "workflow") {
      return workflows.find((workflow) => workflow.id === issue.executorId)?.name || "Workflow"
    }
    return null
  }

  function assigneeLabel(issue: Issue): string {
    const parts = [ownerLabel(issue), executorLabel(issue)].filter((label): label is string => label != null)
    return parts.length > 0 ? parts.join(" · ") : "Unassigned"
  }

  const pageLabel = useMemo(() => {
    if (total === 0) return "0 issues"
    const start = (page - 1) * PAGE_SIZE + 1
    const end = Math.min(page * PAGE_SIZE, total)
    return `${start}-${end} of ${total}`
  }, [page, total])

  function handleCreate(values: {
    title: string
    description?: string
    status: Issue["status"]
    owner_id: string
    executor_kind: "agent" | "workflow" | ""
    executor_id: string
  }) {
    if (!token || !spaceId) return
    setSaving(true)
    setError(null)
    createIssue(spaceId, { title: values.title, description: values.description }, token)
      .then(async (created) => {
        const needsPatch =
          values.status !== "todo" ||
          values.owner_id !== "" ||
          values.executor_kind !== "" ||
          values.executor_id !== ""
        if (needsPatch) {
          await updateIssue(
            spaceId,
            created.id,
            {
              version: created.version,
              status: values.status,
              owner_id: values.owner_id,
              executor_kind: values.executor_kind,
              executor_id: values.executor_id,
            },
            token,
          )
        }
        setCreateOpen(false)
        setPage(1)
        navigate({ name: "issues", spaceId })
        void fetchIssues()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to create issue")))
      .finally(() => setSaving(false))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Issues</h1>
          <p className="page-activity__subtitle">
            Track space work items, ownership, and current progress.
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => {
              setError(null)
              setCreateOpen(true)
            }}
          >
            New Issue
          </button>
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}
      {!canAssignWorkflow ? (
        <p className="page-activity__empty">
          You can create issues and assign people or agents here. Workflow assignment is reserved for space owners and admins.
        </p>
      ) : null}

      <section className="issues-page__panel">
        <div className="issues-page__toolbar">
          <h2 className="issues-page__section-title">All Issues</h2>
          <span className="page-activity__meta">{pageLabel}</span>
        </div>

        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : issues.length === 0 ? (
          <p className="page-activity__empty">
            No issues yet. Create one to track a work item, ownership, and progress for this space.
          </p>
        ) : (
          <ul className="issues-page__list">
            {issues.map((issue) => (
              <li key={issue.id} className="issues-page__list-item">
                <button
                  type="button"
                  className="issues-page__row"
                  onClick={() => {
                    navigate({ name: "issue", spaceId, issueId: issue.id })
                  }}
                >
                  <span className="issues-page__row-main">
                    <span className="issues-page__row-title">{issue.title}</span>
                    <span className="issues-page__row-desc">
                      {issue.description?.trim() || "No description"}
                    </span>
                  </span>
                  <span className="issues-page__row-side">
                    {issue.childCount > 0 ? (
                      <span className="page-activity__meta">
                        {issue.doneChildCount}/{issue.childCount} sub-issues
                      </span>
                    ) : null}
                    {issue.commentCount > 0 ? (
                      <span className="page-activity__meta">
                        {issue.commentCount} comment{issue.commentCount === 1 ? "" : "s"}
                      </span>
                    ) : null}
                    <span className="issues-page__status">{issue.status}</span>
                    <span className="page-activity__meta">
                      {assigneeLabel(issue)}
                    </span>
                    <span className="page-activity__meta">{issue.updatedLabel}</span>
                  </span>
                </button>
                {issue.childCount > 0 ? (
                  <div className="issues-page__children">
                    {/* Children load on expand rather than with the page: a
                        board of parents would otherwise pay for every
                        breakdown nobody opened. */}
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      onClick={() => toggleChildren(issue.id)}
                    >
                      {expanded[issue.id] ? "Hide sub-issues" : "Show sub-issues"}
                    </button>
                    {expanded[issue.id] ? (
                      children[issue.id] === undefined ? (
                        <p className="page-activity__empty">Loading…</p>
                      ) : (
                        <ul className="issues-page__child-list">
                          {children[issue.id].map((child) => (
                            <li key={child.id} className="issues-page__child">
                              <button
                                type="button"
                                className="issues-page__row"
                                onClick={() => navigate({ name: "issue", spaceId, issueId: child.id })}
                              >
                                <span className="issues-page__row-main">
                                  <span className="issues-page__row-title">{child.title}</span>
                                </span>
                                <span className="issues-page__row-side">
                                  <span className="issues-page__status">{child.status}</span>
                                  <span className="page-activity__meta">{assigneeLabel(child)}</span>
                                </span>
                              </button>
                            </li>
                          ))}
                        </ul>
                      )
                    ) : null}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}

        <div className="issues-page__pagination">
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </button>
          <span className="page-activity__meta">
            Page {page} / {totalPages}
          </span>
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
          >
            Next
          </button>
        </div>
      </section>

      <IssueModal
        open={createOpen}
        agents={agents}
        workflows={workflows}
        members={members}
        userId={userId}
        loading={saving}
        allowWorkflowAssignment={canAssignWorkflow}
        error={createOpen ? error : null}
        onClose={() => {
          setCreateOpen(false)
          setError(null)
        }}
        onSubmit={handleCreate}
      />

    </div>
  )
}
