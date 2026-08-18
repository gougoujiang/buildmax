import { useCallback, useEffect, useState } from "react"
import type { Workflow, WorkflowRun, WorkflowStepRun } from "../../lib/types"
import { getErrorMessage } from "../../lib/errorMessage"
import {
  apiWorkflowRunToWorkflowRun,
  apiWorkflowStepRunToWorkflowStepRun,
  apiWorkflowToWorkflow,
} from "../../lib/api/mappers"
import { getWorkflow, getWorkflowRunDetail } from "../../features/workflows"
import { navigate } from "../../router"
import { useTeam } from "../../contexts/TeamContext"

interface WorkflowRunDetailProps {
  token: string | null
  workflowRunId: string
}

export function WorkflowRunDetail({ token, workflowRunId }: WorkflowRunDetailProps) {
  const { currentTeamId } = useTeam()
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [run, setRun] = useState<WorkflowRun | null>(null)
  const [steps, setSteps] = useState<WorkflowStepRun[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastRefreshedAt, setLastRefreshedAt] = useState<number | null>(null)

  const load = useCallback(async (background = false) => {
    if (!token || !currentTeamId) {
      setWorkflow(null)
      setRun(null)
      setSteps([])
      setLoading(false)
      setRefreshing(false)
      return
    }
    if (background) {
      setRefreshing(true)
    } else {
      setLoading(true)
      setError(null)
    }
    try {
      const detail = await getWorkflowRunDetail(currentTeamId, workflowRunId, token)
      const mappedRun = apiWorkflowRunToWorkflowRun(detail.run)
      setRun(mappedRun)
      setSteps(detail.steps.map(apiWorkflowStepRunToWorkflowStepRun))
      const workflowApi = await getWorkflow(currentTeamId, detail.run.workflow_id, token)
      setWorkflow(apiWorkflowToWorkflow(workflowApi))
      setLastRefreshedAt(Date.now())
    } catch (err) {
      if (!background) {
        setError(getErrorMessage(err, "Failed to load workflow run"))
      }
    } finally {
      if (background) {
        setRefreshing(false)
      } else {
        setLoading(false)
      }
    }
  }, [token, currentTeamId, workflowRunId])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (run == null) return
    if (run.status !== "pending" && run.status !== "running") return
    const timer = window.setInterval(() => {
      void load(true)
    }, 3000)
    return () => window.clearInterval(timer)
  }, [run, load])

  const isLive = run?.status === "pending" || run?.status === "running"
  const refreshedLabel = lastRefreshedAt
    ? new Date(lastRefreshedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
    : null

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Workflow Run</h1>
          <p className="page-activity__subtitle">
            Inspect step-by-step workflow execution in a dedicated run view.
          </p>
        </div>
        <div className="page-activity__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={loading || refreshing}
            onClick={() => {
              void load(true)
            }}
          >
            {refreshing ? "Refreshing…" : "Refresh"}
          </button>
          {workflow ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => navigate({ name: "workflow", workflowId: workflow.id })}
            >
              Back to Workflow
            </button>
          ) : null}
          {run?.conversationId ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => navigate({ name: "conversation", conversationId: run.conversationId })}
            >
              Open Conversation
            </button>
          ) : null}
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}

      {loading ? (
        <p className="page-activity__empty">Loading…</p>
      ) : run == null ? (
        <p className="page-activity__empty">Workflow run not found.</p>
      ) : (
        <div className="workflow-run-page__grid">
          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">{workflow?.name ?? run.workflowId}</h2>
              <span className="issues-page__status">{run.status}</span>
            </div>
            <div className="workflow-run-page__meta">
              <div><strong>Run ID:</strong> {run.id}</div>
              <div><strong>Created:</strong> {run.createdLabel}</div>
              {run.startedAt ? <div><strong>Started:</strong> {new Date(run.startedAt * 1000).toLocaleString()}</div> : null}
              {run.endedAt ? <div><strong>Ended:</strong> {new Date(run.endedAt * 1000).toLocaleString()}</div> : null}
              {run.issueId ? <div><strong>Issue ID:</strong> {run.issueId}</div> : null}
              <div><strong>Conversation:</strong> {run.conversationId}</div>
              <div>
                <strong>Mode:</strong> {isLive ? "Live updates enabled" : "Final snapshot"}
              </div>
              {refreshedLabel ? <div><strong>Last refreshed:</strong> {refreshedLabel}</div> : null}
              {run.errorMessage ? <div className="modal__error">{run.errorMessage}</div> : null}
            </div>
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Steps</h2>
              <span className="page-activity__meta">{steps.length} total</span>
            </div>
            {steps.length === 0 ? (
              <p className="page-activity__empty">No steps recorded.</p>
            ) : (
              <ol className="workflow-page__steps">
                {steps.map((step) => (
                  <li key={step.id} className="workflow-page__step">
                    <div className="workflow-page__step-head">
                      <strong>{step.stepId}</strong>
                      <span className="issues-page__status">{step.status}</span>
                    </div>
                    <div className="workflow-page__step-body">
                      <div className="page-activity__meta">{step.stepType}</div>
                      <div>{step.prompt}</div>
                      {step.targetAgentId ? (
                        <div className="page-activity__meta">
                          Agent: {step.agentName ? `${step.agentName} (${step.targetAgentId})` : step.targetAgentId}
                        </div>
                      ) : null}
                      {step.agentInstructions ? (
                        <details className="workflow-run-page__step-agent">
                          <summary className="page-activity__meta">Agent definition used by this step</summary>
                          <pre className="workflow-page__step-output">{step.agentInstructions}</pre>
                        </details>
                      ) : null}
                      {step.taskId ? (
                        <div className="page-activity__meta">
                          Task: {step.taskId}
                          {step.taskRunId ? ` / Run: ${step.taskRunId}` : ""}
                        </div>
                      ) : null}
                      {step.taskId && run.conversationId ? (
                        <div className="workflow-run-page__step-actions">
                          <button
                            type="button"
                            className="page-activity__action-btn"
                            onClick={() => navigate({ name: "conversation", conversationId: run.conversationId })}
                          >
                            Open Conversation Trace
                          </button>
                        </div>
                      ) : null}
                      {step.outputSummary ? <pre className="workflow-page__step-output">{step.outputSummary}</pre> : null}
                      {step.errorMessage ? <p className="modal__error">{step.errorMessage}</p> : null}
                    </div>
                  </li>
                ))}
              </ol>
            )}
          </section>
        </div>
      )}
    </div>
  )
}
