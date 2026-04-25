import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, Issue } from "../lib/types"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import { apiAgentToAgent, apiIssueToIssue } from "../lib/api/mappers"
import { createIssue, getIssue, getIssues, updateIssue } from "../features/issues"
import { getAgents } from "../features/agents"
import { IssueModal } from "../components/IssueModal"

const PAGE_SIZE = 10

interface IssuesProps {
  token: string | null
  routeIssueId?: string
  userId?: string
}

export function Issues({ token, routeIssueId, userId }: IssuesProps) {
  const [issues, setIssues] = useState<Issue[]>([])
  const [total, setTotal] = useState(0)
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [selectedIssue, setSelectedIssue] = useState<Issue | null>(null)

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const fetchIssues = useCallback(() => {
    if (!token) return Promise.resolve()
    setLoading(true)
    setError(null)
    return Promise.all([
      getIssues(token, { limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }),
      getAgents(token),
    ])
      .then(([issueRes, agentRes]) => {
        setIssues(issueRes.issues.map(apiIssueToIssue))
        setTotal(issueRes.total)
        setAgents(agentRes.map(apiAgentToAgent))
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to load issues")))
      .finally(() => setLoading(false))
  }, [page, token])

  useEffect(() => {
    void fetchIssues()
  }, [fetchIssues])

  useEffect(() => {
    if (!token || !routeIssueId) return
    getIssue(routeIssueId, token)
      .then((issue) => setSelectedIssue(apiIssueToIssue(issue)))
      .catch((err) => setError(getErrorMessage(err, "Failed to load issue")))
  }, [routeIssueId, token])

  const pageLabel = useMemo(() => {
    if (total === 0) return "0 issues"
    const start = (page - 1) * PAGE_SIZE + 1
    const end = Math.min(page * PAGE_SIZE, total)
    return `${start}-${end} of ${total}`
  }, [page, total])

  function closeDetailModal() {
    setSelectedIssue(null)
    if (routeIssueId) navigate({ name: "issues" })
  }

  function handleCreate(values: { title: string; description?: string }) {
    if (!token) return
    setSaving(true)
    setError(null)
    createIssue(values, token)
      .then(() => {
        setCreateOpen(false)
        setPage(1)
        setSelectedIssue(null)
        navigate({ name: "issues" })
        void fetchIssues()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to create issue")))
      .finally(() => setSaving(false))
  }

  function handleSave(values: {
    title: string
    description: string
    status: Issue["status"]
    assignee_kind: "person" | "agent" | ""
    assignee_id: string
  }) {
    if (!token || !selectedIssue) return
    setSaving(true)
    setError(null)
    updateIssue(
      selectedIssue.id,
      {
        title: values.title,
        description: values.description,
        status: values.status,
        assignee_kind: values.assignee_kind,
        assignee_id: values.assignee_id,
      },
      token,
    )
      .then((updated) => {
        const mapped = apiIssueToIssue(updated)
        setIssues((prev) => prev.map((issue) => (issue.id === mapped.id ? mapped : issue)))
        setSelectedIssue(null)
        navigate({ name: "issues" })
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to update issue")))
      .finally(() => setSaving(false))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Issues</h1>
          <p className="page-activity__subtitle">
            Track top-level work items, ownership, and current progress.
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

      <section className="issues-page__panel">
        <div className="issues-page__toolbar">
          <h2 className="issues-page__section-title">All Issues</h2>
          <span className="page-activity__meta">{pageLabel}</span>
        </div>

        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : issues.length === 0 ? (
          <p className="page-activity__empty">No issues yet.</p>
        ) : (
          <ul className="issues-page__list">
            {issues.map((issue) => (
              <li key={issue.id} className="issues-page__list-item">
                <button
                  type="button"
                  className="issues-page__row"
                  onClick={() => {
                    setSelectedIssue(issue)
                    navigate({ name: "issue", issueId: issue.id })
                  }}
                >
                  <span className="issues-page__row-main">
                    <span className="issues-page__row-title">{issue.title}</span>
                    <span className="issues-page__row-desc">
                      {issue.description?.trim() || "No description"}
                    </span>
                  </span>
                  <span className="issues-page__row-side">
                    <span className="issues-page__status">{issue.status}</span>
                    <span className="page-activity__meta">
                      {issue.assigneeKind === "person"
                        ? "Me"
                        : issue.assigneeKind === "agent"
                          ? agents.find((agent) => agent.id === issue.assigneeId)?.name || "Agent"
                          : "Unassigned"}
                    </span>
                    <span className="page-activity__meta">{issue.updatedLabel}</span>
                  </span>
                </button>
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
        mode="create"
        agents={agents}
        userId={userId}
        loading={saving}
        error={createOpen ? error : null}
        onClose={() => {
          setCreateOpen(false)
          setError(null)
        }}
        onSubmit={({ title, description }) => handleCreate({ title, description })}
      />

      <IssueModal
        open={selectedIssue != null}
        mode="edit"
        issue={selectedIssue}
        agents={agents}
        userId={userId}
        loading={saving}
        error={selectedIssue != null ? error : null}
        onClose={() => {
          setError(null)
          closeDetailModal()
        }}
        onSubmit={handleSave}
      />
    </div>
  )
}
