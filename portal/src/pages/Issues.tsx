import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, Issue } from "../lib/types"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import { apiAgentToAgent, apiIssueToIssue, apiWorkflowToWorkflow } from "../lib/api/mappers"
import { createIssue, getIssues } from "../features/issues"
import { getAgents } from "../features/agents"
import { getTeamMembers } from "../features/teams/api"
import { getWorkflows } from "../features/workflows"
import { IssueModal } from "../components/IssueModal"
import { useTeam } from "../contexts/TeamContext"
import type { ApiTeamMember } from "../lib/api/types"
import type { Workflow } from "../lib/types"

const PAGE_SIZE = 10

interface IssuesProps {
  token: string | null
  userId?: string
}

export function Issues({ token, userId }: IssuesProps) {
  const { currentTeamId } = useTeam()
  const [issues, setIssues] = useState<Issue[]>([])
  const [total, setTotal] = useState(0)
  const [agents, setAgents] = useState<Agent[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [members, setMembers] = useState<ApiTeamMember[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const fetchIssues = useCallback(() => {
    if (!token || !currentTeamId) {
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
      getIssues(currentTeamId, token, { limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }),
      getAgents(currentTeamId, token),
      getTeamMembers(currentTeamId, token),
      getWorkflows(currentTeamId, token),
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
  }, [page, token, currentTeamId])

  useEffect(() => {
    void fetchIssues()
  }, [fetchIssues])

  useEffect(() => {
    setPage(1)
  }, [currentTeamId])

  function assigneeLabel(issue: Issue): string {
    if (issue.assigneeKind === "person") {
      if (issue.assigneeId === userId) return "Me"
      const member = members.find((item) => item.user_id === issue.assigneeId)
      if (member?.user_name) return member.user_name
      if (member?.user_email) return member.user_email
      if (member) return `Member ${member.user_id.slice(0, 8)}`
      return "Member"
    }
    if (issue.assigneeKind === "agent") {
      return agents.find((agent) => agent.id === issue.assigneeId)?.name || "Agent"
    }
    if (issue.assigneeKind === "workflow") {
      return workflows.find((workflow) => workflow.id === issue.assigneeId)?.name || "Workflow"
    }
    return "Unassigned"
  }

  const pageLabel = useMemo(() => {
    if (total === 0) return "0 issues"
    const start = (page - 1) * PAGE_SIZE + 1
    const end = Math.min(page * PAGE_SIZE, total)
    return `${start}-${end} of ${total}`
  }, [page, total])

  function handleCreate(values: { title: string; description?: string }) {
    if (!token || !currentTeamId) return
    setSaving(true)
    setError(null)
    createIssue(currentTeamId, values, token)
      .then(() => {
        setCreateOpen(false)
        setPage(1)
        navigate({ name: "issues" })
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
            Track team work items, ownership, and current progress.
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
                      {assigneeLabel(issue)}
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
        workflows={workflows}
        members={members}
        userId={userId}
        loading={saving}
        error={createOpen ? error : null}
        onClose={() => {
          setCreateOpen(false)
          setError(null)
        }}
        onSubmit={({ title, description }) => handleCreate({ title, description })}
      />

    </div>
  )
}
