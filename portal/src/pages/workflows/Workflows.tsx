import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, Workflow } from "../../lib/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import {
  apiAgentToAgent,
  apiWorkflowToWorkflow,
} from "../../lib/api/mappers"
import { getAgents } from "../../features/agents"
import {
  createWorkflow,
  getWorkflows,
} from "../../features/workflows"
import { WorkflowModal } from "../../components/WorkflowModal"
import { useTeam } from "../../contexts/TeamContext"

interface WorkflowsProps {
  token: string | null
}

export function Workflows({ token }: WorkflowsProps) {
  const { currentTeamId, currentUserRole } = useTeam()
  const [agents, setAgents] = useState<Agent[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const canManageWorkflows = currentUserRole === "owner" || currentUserRole === "admin"

  const fetchWorkflows = useCallback(() => {
    if (!token || !currentTeamId) {
      setAgents([])
      setWorkflows([])
      setLoading(false)
      return Promise.resolve()
    }
    setLoading(true)
    setError(null)
    return Promise.all([
      getWorkflows(currentTeamId, token),
      getAgents(currentTeamId, token),
    ])
      .then(([workflowRes, agentRes]) => {
        setWorkflows(workflowRes.workflows.map(apiWorkflowToWorkflow))
        setAgents(agentRes.map(apiAgentToAgent))
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to load workflows")))
      .finally(() => setLoading(false))
  }, [token, currentTeamId])

  useEffect(() => {
    void fetchWorkflows()
  }, [fetchWorkflows])

  const workflowCountLabel = useMemo(() => {
    if (workflows.length === 0) return "0 workflows"
    if (workflows.length === 1) return "1 workflow"
    return `${workflows.length} workflows`
  }, [workflows.length])

  function handleCreate(values: { name: string; description: string; definition: string }) {
    if (!token || !currentTeamId) return
    setSaving(true)
    setError(null)
    createWorkflow(currentTeamId, values, token)
      .then((created) => {
        setCreateOpen(false)
        setWorkflows((prev) => [...prev, apiWorkflowToWorkflow(created)])
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to create workflow")))
      .finally(() => setSaving(false))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Workflows</h1>
          <p className="page-activity__subtitle">
            Define reusable step-by-step execution plans and run them manually.
          </p>
        </div>
        <div className="page-activity__actions">
          {canManageWorkflows ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => {
                setError(null)
                setCreateOpen(true)
              }}
            >
              New Workflow
            </button>
          ) : null}
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}
      {!canManageWorkflows ? (
        <p className="page-activity__empty">
          You can view workflows here, but only team owners and admins can create or edit them.
        </p>
      ) : null}

      <section className="issues-page__panel">
        <div className="issues-page__toolbar">
          <h2 className="issues-page__section-title">All Workflows</h2>
          <span className="page-activity__meta">{workflowCountLabel}</span>
        </div>

        {loading ? (
          <p className="page-activity__empty">Loading…</p>
        ) : workflows.length === 0 ? (
          <p className="page-activity__empty">
            {canManageWorkflows
              ? "No workflows yet. Create one to define a reusable execution plan for this team."
              : "No workflows are available in this team yet. Team owners and admins can publish one when a shared process is ready."}
          </p>
        ) : (
          <ul className="issues-page__list">
            {workflows.map((workflow) => (
              <li key={workflow.id} className="issues-page__list-item">
                <button
                  type="button"
                  className="issues-page__row"
                  onClick={() => navigate({ name: "workflow", workflowId: workflow.id })}
                >
                  <span className="issues-page__row-main">
                    <span className="issues-page__row-title">{workflow.name}</span>
                    <span className="issues-page__row-desc">
                      {workflow.description?.trim() || "No description"}
                    </span>
                  </span>
                  <span className="issues-page__row-side">
                    <span className="issues-page__status">{workflow.status}</span>
                    <span className="page-activity__meta">{workflow.updatedLabel}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <WorkflowModal
        open={createOpen}
        mode="create"
        agents={agents}
        loading={saving}
        running={false}
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
