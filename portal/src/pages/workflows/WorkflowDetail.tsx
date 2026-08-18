import { useCallback, useEffect, useState } from "react"
import type { Agent, Workflow, WorkflowRevision, WorkflowRun } from "../../lib/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import {
  apiAgentToAgent,
  apiWorkflowRevisionToWorkflowRevision,
  apiWorkflowRunToWorkflowRun,
  apiWorkflowToWorkflow,
} from "../../lib/api/mappers"
import { getAgents } from "../../features/agents"
import {
  getWorkflow,
  getWorkflowRevisions,
  getWorkflowRuns,
  restoreWorkflowRevision,
  runWorkflow,
  updateWorkflow,
} from "../../features/workflows"
import { RevisionHistory } from "../../components/RevisionHistory"
import { useTeam } from "../../contexts/TeamContext"

interface WorkflowStepDraft {
  step_id: string
  type: string
  target_agent_id: string
  prompt: string
}

interface ParsedWorkflowDefinition {
  steps: WorkflowStepDraft[]
}

interface WorkflowDetailProps {
  token: string | null
  workflowId: string
}

function buildDefinitionFromSteps(steps: WorkflowStepDraft[]) {
  return JSON.stringify({ steps }, null, 2)
}

function parseWorkflowDefinition(definition: string): ParsedWorkflowDefinition | null {
  try {
    const parsed = JSON.parse(definition) as { steps?: unknown }
    if (!Array.isArray(parsed.steps)) return null
    return {
      steps: parsed.steps.map((step, index) => {
        const record = typeof step === "object" && step != null ? (step as Record<string, unknown>) : {}
        return {
          step_id: typeof record.step_id === "string" && record.step_id.trim()
            ? record.step_id
            : `step_${index + 1}`,
          type: typeof record.type === "string" && record.type.trim() ? record.type : "agent_task",
          target_agent_id: typeof record.target_agent_id === "string" ? record.target_agent_id : "",
          prompt: typeof record.prompt === "string" ? record.prompt : "",
        }
      }),
    }
  } catch {
    return null
  }
}

function buildDefaultStep(agentId = "", index = 0): WorkflowStepDraft {
  return {
    step_id: `step_${index + 1}`,
    type: "agent_task",
    target_agent_id: agentId,
    prompt: "Describe what this step should do.",
  }
}

export function WorkflowDetail({ token, workflowId }: WorkflowDetailProps) {
  const { currentTeamId, currentUserRole } = useTeam()
  const [agents, setAgents] = useState<Agent[]>([])
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [runs, setRuns] = useState<WorkflowRun[]>([])
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [definition, setDefinition] = useState("")
  const [status, setStatus] = useState<Workflow["status"]>("draft")
  const [stepDrafts, setStepDrafts] = useState<WorkflowStepDraft[]>([])
  const [definitionHint, setDefinitionHint] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [revisions, setRevisions] = useState<WorkflowRevision[]>([])
  const [revisionsLoading, setRevisionsLoading] = useState(false)
  const [revisionsError, setRevisionsError] = useState<string | null>(null)
  const [restoringRevision, setRestoringRevision] = useState<number | null>(null)
  const canManageWorkflows = currentUserRole === "owner" || currentUserRole === "admin"

  const syncDefinition = useCallback((nextSteps: WorkflowStepDraft[]) => {
    setStepDrafts(nextSteps)
    setDefinition(buildDefinitionFromSteps(nextSteps))
    setDefinitionHint(null)
  }, [])

  const load = useCallback(async () => {
    if (!token || !currentTeamId) {
      setAgents([])
      setWorkflow(null)
      setRuns([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [workflowApi, runsApi, agentsApi, revisionsApi] = await Promise.all([
        getWorkflow(currentTeamId, workflowId, token),
        getWorkflowRuns(currentTeamId, workflowId, token),
        getAgents(currentTeamId, token),
        getWorkflowRevisions(currentTeamId, workflowId, token),
      ])
      const mappedWorkflow = apiWorkflowToWorkflow(workflowApi)
      const parsed = parseWorkflowDefinition(mappedWorkflow.definition)
      setWorkflow(mappedWorkflow)
      setRuns(runsApi.runs.map(apiWorkflowRunToWorkflowRun))
      setAgents(agentsApi.map(apiAgentToAgent))
      setRevisions(revisionsApi.revisions.map(apiWorkflowRevisionToWorkflowRevision))
      setName(mappedWorkflow.name)
      setDescription(mappedWorkflow.description)
      setDefinition(mappedWorkflow.definition)
      setStatus(mappedWorkflow.status)
      setStepDrafts(parsed?.steps ?? [])
      setDefinitionHint(parsed ? null : "Definition JSON is invalid, so the step form is temporarily disabled.")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load workflow"))
    } finally {
      setLoading(false)
    }
  }, [token, currentTeamId, workflowId])

  const loadRevisions = useCallback(() => {
    if (!token || !currentTeamId) return
    setRevisionsLoading(true)
    setRevisionsError(null)
    getWorkflowRevisions(currentTeamId, workflowId, token)
      .then((res) => setRevisions(res.revisions.map(apiWorkflowRevisionToWorkflowRevision)))
      .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to load history")))
      .finally(() => setRevisionsLoading(false))
  }, [token, currentTeamId, workflowId])

  useEffect(() => {
    void load()
  }, [load])

  function handleSave() {
    if (!token || !currentTeamId || !workflow || !canManageWorkflows) return
    setSaving(true)
    setError(null)
    updateWorkflow(
      currentTeamId,
      workflow.id,
      { name: name.trim(), description, definition: definition.trim(), status },
      token,
    )
      .then((updated) => {
        const mapped = apiWorkflowToWorkflow(updated)
        const parsed = parseWorkflowDefinition(mapped.definition)
        setWorkflow(mapped)
        setName(mapped.name)
        setDescription(mapped.description)
        setDefinition(mapped.definition)
        setStatus(mapped.status)
        setStepDrafts(parsed?.steps ?? [])
        setDefinitionHint(parsed ? null : "Definition JSON is invalid, so the step form is temporarily disabled.")
        loadRevisions()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to update workflow")))
      .finally(() => setSaving(false))
  }

  function handleRestoreRevision(revision: number) {
    if (!token || !currentTeamId || !workflow || !canManageWorkflows) return
    setRevisionsError(null)
    setRestoringRevision(revision)
    restoreWorkflowRevision(currentTeamId, workflow.id, revision, token)
      .then((restored) => {
        const mapped = apiWorkflowToWorkflow(restored)
        const parsed = parseWorkflowDefinition(mapped.definition)
        setWorkflow(mapped)
        setName(mapped.name)
        setDescription(mapped.description)
        setDefinition(mapped.definition)
        setStatus(mapped.status)
        setStepDrafts(parsed?.steps ?? [])
        setDefinitionHint(parsed ? null : "Definition JSON is invalid, so the step form is temporarily disabled.")
        loadRevisions()
      })
      .catch((err) => setRevisionsError(getErrorMessage(err, "Failed to restore revision")))
      .finally(() => setRestoringRevision(null))
  }

  function handleRunWorkflow() {
    if (!token || !currentTeamId || !workflow) return
    setRunning(true)
    setError(null)
    runWorkflow(currentTeamId, workflow.id, token)
      .then((detail) => {
        const mappedRun = apiWorkflowRunToWorkflowRun(detail.run)
        setRuns((prev) => [mappedRun, ...prev.filter((run) => run.id !== mappedRun.id)])
        navigate({ name: "workflowRun", workflowRunId: mappedRun.id })
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to run workflow")))
      .finally(() => setRunning(false))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Workflow Detail</h1>
          <p className="page-activity__subtitle">
            Edit the workflow definition, run it manually, and inspect recent executions.
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            onClick={() => navigate({ name: "workflows" })}
          >
            Back to Workflows
          </button>
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={loading}
            onClick={() => {
              void load()
            }}
          >
            Refresh
          </button>
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={running || loading || workflow == null || workflow.status !== "published"}
            onClick={handleRunWorkflow}
          >
            {running ? "Running…" : "Run Workflow"}
          </button>
          {canManageWorkflows ? (
            <button
              type="button"
              className="page-activity__action-btn"
              disabled={saving || loading || workflow == null || !name.trim() || !definition.trim()}
              onClick={handleSave}
            >
              {saving ? "Saving…" : "Save"}
            </button>
          ) : null}
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}
      {!canManageWorkflows ? (
        <p className="page-activity__empty">
          This workflow is read-only for your role. You can still inspect it here, and you can run it when it is `published`.
        </p>
      ) : null}

      {loading ? (
        <p className="page-activity__empty">Loading…</p>
      ) : workflow == null ? (
        <p className="page-activity__empty">Workflow not found.</p>
      ) : (
        <div className="workflow-detail-page__grid">
          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Definition</h2>
              <span className="page-activity__meta">{workflow.id}</span>
            </div>

            <div className="workflow-page__form">
              <label className="issues-page__field">
                <span className="issues-page__field-label">Status</span>
                <select
                  className="issues-page__input"
                  value={status}
                  disabled={!canManageWorkflows}
                  onChange={(e) => setStatus(e.target.value as Workflow["status"])}
                >
                  <option value="draft">Draft</option>
                  <option value="published">Published</option>
                  <option value="archived">Archived</option>
                </select>
                <span className="issues-page__field-label">
                  Only `published` workflows can be assigned to issues or run manually.
                </span>
              </label>
              <label className="issues-page__field">
                <span className="issues-page__field-label">Name</span>
                <input className="issues-page__input" value={name} disabled={!canManageWorkflows} onChange={(e) => setName(e.target.value)} />
              </label>
              <label className="issues-page__field">
                <span className="issues-page__field-label">Description</span>
                <textarea
                  className="issues-page__textarea"
                  rows={4}
                  value={description}
                  disabled={!canManageWorkflows}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </label>
              {workflow.status !== "published" ? (
                <p className="page-activity__empty">
                  This workflow is currently `{workflow.status}`. Publish it before manual runs or issue assignment.
                </p>
              ) : null}

              <section className="workflow-page__builder">
                <div className="issues-page__toolbar">
                  <h3 className="issues-page__section-title">Steps</h3>
                  {canManageWorkflows ? (
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      onClick={() => syncDefinition([...stepDrafts, buildDefaultStep(agents[0]?.id ?? "", stepDrafts.length)])}
                    >
                      Add Step
                    </button>
                  ) : null}
                </div>
                {stepDrafts.length === 0 ? (
                  <p className="page-activity__empty">No steps yet.</p>
                ) : (
                  <ol className="workflow-page__steps">
                    {stepDrafts.map((step, index) => (
                      <li key={`${step.step_id}-${index}`} className="workflow-page__step">
                        <div className="workflow-page__step-head">
                          <strong>Step {index + 1}</strong>
                          {canManageWorkflows ? (
                            <button
                              type="button"
                              className="page-activity__action-btn"
                              disabled={stepDrafts.length === 1}
                              onClick={() => syncDefinition(stepDrafts.filter((_, draftIndex) => draftIndex !== index))}
                            >
                              Remove
                            </button>
                          ) : null}
                        </div>
                        <div className="workflow-page__builder-grid">
                          <label className="issues-page__field">
                            <span className="issues-page__field-label">Step ID</span>
                            <input
                              className="issues-page__input"
                              value={step.step_id}
                              disabled={!canManageWorkflows}
                              onChange={(e) =>
                                syncDefinition(
                                  stepDrafts.map((draft, draftIndex) =>
                                    draftIndex === index ? { ...draft, step_id: e.target.value } : draft,
                                  ),
                                )
                              }
                            />
                          </label>
                          <label className="issues-page__field">
                            <span className="issues-page__field-label">Type</span>
                            <input
                              className="issues-page__input"
                              value={step.type}
                              disabled={!canManageWorkflows}
                              onChange={(e) =>
                                syncDefinition(
                                  stepDrafts.map((draft, draftIndex) =>
                                    draftIndex === index ? { ...draft, type: e.target.value } : draft,
                                  ),
                                )
                              }
                            />
                          </label>
                        </div>
                        <label className="issues-page__field">
                          <span className="issues-page__field-label">Target Agent</span>
                          <select
                            className="issues-page__input"
                            value={step.target_agent_id}
                            disabled={!canManageWorkflows}
                            onChange={(e) =>
                              syncDefinition(
                                stepDrafts.map((draft, draftIndex) =>
                                  draftIndex === index ? { ...draft, target_agent_id: e.target.value } : draft,
                                ),
                              )
                            }
                          >
                            <option value="">Select an agent</option>
                            {agents.map((agent) => (
                              <option key={agent.id} value={agent.id}>
                                {agent.name} ({agent.id})
                              </option>
                            ))}
                          </select>
                        </label>
                        <label className="issues-page__field">
                          <span className="issues-page__field-label">Prompt</span>
                          <textarea
                            className="issues-page__textarea"
                            rows={4}
                            value={step.prompt}
                            disabled={!canManageWorkflows}
                            onChange={(e) =>
                              syncDefinition(
                                stepDrafts.map((draft, draftIndex) =>
                                  draftIndex === index ? { ...draft, prompt: e.target.value } : draft,
                                ),
                              )
                            }
                          />
                        </label>
                      </li>
                    ))}
                  </ol>
                )}
              </section>

              <label className="issues-page__field">
                <span className="issues-page__field-label">Definition (JSON)</span>
                <textarea
                  className="issues-page__textarea workflow-page__definition"
                  rows={12}
                  value={definition}
                  disabled={!canManageWorkflows}
                  onChange={(e) => {
                    const nextDefinition = e.target.value
                    setDefinition(nextDefinition)
                    const parsed = parseWorkflowDefinition(nextDefinition)
                    if (!parsed) {
                      setDefinitionHint("JSON is invalid right now. Fix it to re-sync the step editor.")
                      return
                    }
                    setStepDrafts(parsed.steps)
                    setDefinitionHint(null)
                  }}
                />
                {definitionHint ? <span className="issues-page__field-label">{definitionHint}</span> : null}
              </label>
            </div>
          </section>

          <section className="issues-page__panel">
            <RevisionHistory
              title="History"
              entries={revisions.map((rev) => ({
                id: rev.id,
                revision: rev.revision,
                createdBy: rev.createdBy,
                createdLabel: rev.createdLabel,
                summary: `${rev.name} · ${rev.status}`,
              }))}
              currentRevision={workflow?.revision ?? 0}
              loading={revisionsLoading}
              error={revisionsError}
              canRestore={canManageWorkflows}
              restoringRevision={restoringRevision}
              onRestore={handleRestoreRevision}
            />
            <p className="page-activity__meta">
              Restoring writes that version's name, description, and steps back as a new
              version. The lifecycle state is left as it is.
            </p>
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Recent Runs</h2>
              <span className="page-activity__meta">{runs.length} total</span>
            </div>
            {runs.length === 0 ? (
              <p className="page-activity__empty">No runs yet.</p>
            ) : (
              <ul className="workflow-page__runs">
                {runs.map((run) => (
                  <li key={run.id}>
                    <button
                      type="button"
                      className="workflow-page__run-row"
                      onClick={() => navigate({ name: "workflowRun", workflowRunId: run.id })}
                    >
                      <span>
                        <strong>{run.status}</strong>
                        <span className="page-activity__meta workflow-detail-page__run-id">{run.id}</span>
                      </span>
                      <span className="page-activity__meta">{run.createdLabel}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      )}
    </div>
  )
}
