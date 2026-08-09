import { useEffect, useState } from "react"
import { BaseModal } from "@buildmax/gui"
import type { Agent, Workflow, WorkflowRun } from "../lib/types"

interface WorkflowStepDraft {
  step_id: string
  type: string
  target_agent_id: string
  prompt: string
}

interface ParsedWorkflowDefinition {
  steps: WorkflowStepDraft[]
}

interface WorkflowModalProps {
  open: boolean
  mode: "create" | "edit"
  agents?: Agent[]
  workflow?: Workflow | null
  runs?: WorkflowRun[]
  loading: boolean
  running: boolean
  error: string | null
  onClose: () => void
  onSubmit: (values: { name: string; description: string; definition: string }) => void
  onRunWorkflow?: () => void
  onSelectRun?: (runId: string) => void
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

export function WorkflowModal({
  open,
  mode,
  agents = [],
  workflow,
  runs = [],
  loading,
  running,
  error,
  onClose,
  onSubmit,
  onRunWorkflow,
  onSelectRun,
}: WorkflowModalProps) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [definition, setDefinition] = useState("")
  const [stepDrafts, setStepDrafts] = useState<WorkflowStepDraft[]>([])
  const [definitionHint, setDefinitionHint] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    if (mode === "edit" && workflow) {
      setName(workflow.name)
      setDescription(workflow.description)
      setDefinition(workflow.definition)
      const parsed = parseWorkflowDefinition(workflow.definition)
      setStepDrafts(parsed?.steps ?? [])
      setDefinitionHint(parsed ? null : "Definition JSON is invalid, so the step form is temporarily disabled.")
      return
    }
    setName("")
    setDescription("")
    const defaultAgentId = agents[0]?.id ?? ""
    const defaultSteps = [buildDefaultStep(defaultAgentId)]
    setStepDrafts(defaultSteps)
    setDefinition(buildDefinitionFromSteps(defaultSteps))
    setDefinitionHint(null)
  }, [open, mode, workflow, agents])

  const titleText = mode === "create" ? "New Workflow" : "Workflow Details"
  const submitText = mode === "create" ? "Create workflow" : "Save"

  function syncDefinition(nextSteps: WorkflowStepDraft[]) {
    setStepDrafts(nextSteps)
    setDefinition(buildDefinitionFromSteps(nextSteps))
    setDefinitionHint(null)
  }

  return (
    <BaseModal
      open={open}
      title={titleText}
      titleId="workflow-modal-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <div className="workflow-page__form">
          <label className="issues-page__field">
            <span className="issues-page__field-label">Name</span>
            <input className="issues-page__input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Customer research workflow" />
          </label>
          <label className="issues-page__field">
            <span className="issues-page__field-label">Description</span>
            <textarea className="issues-page__textarea" rows={4} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What this workflow does" />
          </label>
          <section className="workflow-page__builder">
            <div className="issues-page__toolbar">
              <h3 className="issues-page__section-title">Steps</h3>
              <button
                type="button"
                className="page-activity__action-btn"
                onClick={() => syncDefinition([...stepDrafts, buildDefaultStep(agents[0]?.id ?? "", stepDrafts.length)])}
              >
                Add Step
              </button>
            </div>
            {stepDrafts.length === 0 ? (
              <p className="page-activity__empty">No steps yet.</p>
            ) : (
              <ol className="workflow-page__steps">
                {stepDrafts.map((step, index) => (
                  <li key={`${step.step_id}-${index}`} className="workflow-page__step">
                    <div className="workflow-page__step-head">
                      <strong>Step {index + 1}</strong>
                      <button
                        type="button"
                        className="page-activity__action-btn"
                        disabled={stepDrafts.length === 1}
                        onClick={() => syncDefinition(stepDrafts.filter((_, draftIndex) => draftIndex !== index))}
                      >
                        Remove
                      </button>
                    </div>
                    <div className="workflow-page__builder-grid">
                      <label className="issues-page__field">
                        <span className="issues-page__field-label">Step ID</span>
                        <input
                          className="issues-page__input"
                          value={step.step_id}
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
              rows={10}
              value={definition}
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
              placeholder='{"steps":[...]}'
            />
            {mode === "create" ? (
              <span className="issues-page__field-label">
                {agents.length > 0
                  ? `Template uses agent ${agents[0].name} by default. Replace target_agent_id if needed.`
                  : "Create at least one agent first, then use its id as target_agent_id."}
              </span>
            ) : null}
            {definitionHint ? <span className="issues-page__field-label">{definitionHint}</span> : null}
          </label>

          {mode === "edit" ? (
            <div className="workflow-page__detail-grid">
              <section className="workflow-page__detail-panel">
                <div className="issues-page__toolbar">
                  <h3 className="issues-page__section-title">Runs</h3>
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    disabled={running}
                    onClick={onRunWorkflow}
                  >
                    {running ? "Running…" : "Run Workflow"}
                  </button>
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
                          onClick={() => onSelectRun?.(run.id)}
                        >
                          <span>{run.status}</span>
                          <span className="page-activity__meta">{run.createdLabel}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
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
              disabled={loading || !name.trim() || !definition.trim()}
              onClick={() => onSubmit({ name: name.trim(), description, definition: definition.trim() })}
            >
              {loading ? `${submitText}…` : submitText}
            </button>
          </div>
        </div>
      </div>
    </BaseModal>
  )
}
