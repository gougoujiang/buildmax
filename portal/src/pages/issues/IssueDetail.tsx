import { useCallback, useEffect, useMemo, useState } from "react"
import type { Agent, Issue, IssueFlow, IssueFlowRun, IssueOutput, Workflow } from "../../lib/types"
import type { ApiIssueComment, ApiIssueFlowResponse, ApiTeamMember } from "../../lib/api/types"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import { ApiRequestError } from "../../lib/api/client"
import { taskIsRetryable, taskIsStoppable } from "../../lib/taskStatus"
import {
  apiAgentToAgent,
  apiIssueOutputToIssueOutput,
  apiIssueToIssue,
  apiTaskToTask,
  apiWorkflowRunToWorkflowRun,
  apiWorkflowStepRunToWorkflowStepRun,
  apiWorkflowToWorkflow,
} from "../../lib/api/mappers"
import { getAgents } from "../../features/agents"
import { cancelTask, retryTask } from "../../features/tasks"
import {
  createIssue,
  getIssueFlow,
  IssueDiscussion,
  OutputsList,
  OutputViewerModal,
  runIssueAgent,
  updateIssue,
} from "../../features/issues"
import { RunTraceModal } from "../../features/runs"
import { getTeamMembers } from "../../features/teams/api"
import { getWorkflows, runIssueWorkflow } from "../../features/workflows"
import { useTeam } from "../../contexts/TeamContext"

interface IssueDetailProps {
  token: string | null
  issueId: string
  userId?: string
}

interface TimelineEvent {
  id: string
  label: string
  detail: string
  timestamp: string
  status?: string
}

function mapIssueFlow(api: ApiIssueFlowResponse): IssueFlow {
  return {
    issue: apiIssueToIssue(api.issue),
    parent: api.parent ? apiIssueToIssue(api.parent) : null,
    children: (api.children ?? []).map(apiIssueToIssue),
    workflow: api.workflow ? apiWorkflowToWorkflow(api.workflow) : null,
    runs: api.runs.map((item) => ({
      run: apiWorkflowRunToWorkflowRun(item.run),
      steps: item.steps.map(apiWorkflowStepRunToWorkflowStepRun),
    })),
    agentTasks: api.agent_tasks.map(apiTaskToTask),
    latestResult: api.latest_result ? apiIssueOutputToIssueOutput(api.latest_result) : null,
    outputs: (api.outputs ?? []).map(apiIssueOutputToIssueOutput),
    total: api.total,
  }
}

function formatTimestamp(rfc3339: string): string {
  return new Date(rfc3339).toLocaleString()
}

function latestRun(flow: IssueFlow | null): IssueFlowRun | null {
  return flow?.runs[0] ?? null
}

export function IssueDetail({ token, issueId, userId }: IssueDetailProps) {
  const { currentTeamId, currentUserRole } = useTeam()
  const [flow, setFlow] = useState<IssueFlow | null>(null)
  const [traceRunId, setTraceRunId] = useState<string | null>(null)
  const [agents, setAgents] = useState<Agent[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [members, setMembers] = useState<ApiTeamMember[]>([])
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [status, setStatus] = useState<Issue["status"]>("todo")
  const [assigneeValue, setAssigneeValue] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [runningWorkflow, setRunningWorkflow] = useState(false)
  const [runningAgent, setRunningAgent] = useState(false)
  const [cancelingTaskId, setCancelingTaskId] = useState<string | null>(null)
  const [retryingTaskId, setRetryingTaskId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [viewerOutput, setViewerOutput] = useState<IssueOutput | null>(null)
  const [subIssueTitle, setSubIssueTitle] = useState("")
  const [addingSubIssue, setAddingSubIssue] = useState(false)
  // Owned by the Discussion panel's fetch and mirrored here so the timeline can
  // interleave what people said with what runs did.
  const [comments, setComments] = useState<ApiIssueComment[]>([])
  const canAssignWorkflow = currentUserRole === "owner" || currentUserRole === "admin"

  const load = useCallback(async () => {
    if (!token || !currentTeamId) {
      setFlow(null)
      setAgents([])
      setWorkflows([])
      setMembers([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [flowApi, agentsApi, membersApi, workflowsApi] = await Promise.all([
        getIssueFlow(currentTeamId, issueId, token),
        getAgents(currentTeamId, token),
        getTeamMembers(currentTeamId, token),
        getWorkflows(currentTeamId, token),
      ])
      const mapped = mapIssueFlow(flowApi)
      setFlow(mapped)
      setAgents(agentsApi.map(apiAgentToAgent))
      setMembers(membersApi)
      setWorkflows(workflowsApi.workflows.map(apiWorkflowToWorkflow))
      setTitle(mapped.issue.title)
      setDescription(mapped.issue.description)
      setStatus(mapped.issue.status)
      setAssigneeValue(
        mapped.issue.assigneeKind && mapped.issue.assigneeId
          ? `${mapped.issue.assigneeKind}:${mapped.issue.assigneeId}`
          : "",
      )
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load issue detail"))
    } finally {
      setLoading(false)
    }
  }, [token, currentTeamId, issueId])

  useEffect(() => {
    void load()
  }, [load])

  const currentRun = latestRun(flow)
  const currentRunLatestTaskId =
    [...(currentRun?.steps ?? [])].reverse().find((step) => step.taskId)?.taskId ?? null
  const latestAgentTask = flow?.agentTasks[0] ?? null
  const isWorkflowAssigned = flow?.issue.assigneeKind === "workflow" && Boolean(flow.issue.assigneeId)
  const isAgentAssigned = flow?.issue.assigneeKind === "agent" && Boolean(flow.issue.assigneeId)
  const assignedWorkflowStatus =
    flow?.workflow?.status ??
    workflows.find((workflow) => workflow.id === flow?.issue.assigneeId)?.status
  const publishedAssignableWorkflows = workflows.filter(
    (workflow) => workflow.status === "published" || workflow.id === flow?.issue.assigneeId,
  )

  const openChildCount = (flow?.issue.childCount ?? 0) - (flow?.issue.doneChildCount ?? 0)

  const agentNames = useMemo(() => {
    const out: Record<string, string> = {}
    for (const agent of agents) out[agent.id] = agent.name
    return out
  }, [agents])

  const assigneeLabel = useCallback((issue: Issue): string => {
    if (issue.assigneeKind === "person") {
      if (issue.assigneeId === userId) return "Me"
      const member = members.find((item) => item.user_id === issue.assigneeId)
      if (member?.user_name) return member.user_name
      if (member?.user_email) return member.user_email
      return member ? `Member ${member.user_id.slice(0, 8)}` : "Member"
    }
    if (issue.assigneeKind === "agent") {
      return agents.find((agent) => agent.id === issue.assigneeId)?.name || "Agent"
    }
    if (issue.assigneeKind === "workflow") {
      return flow?.workflow?.name || workflows.find((workflow) => workflow.id === issue.assigneeId)?.name || "Workflow"
    }
    return "Unassigned"
  }, [agents, flow?.workflow?.name, members, userId, workflows])

  const timeline = useMemo<TimelineEvent[]>(() => {
    if (!flow) return []
    const events: TimelineEvent[] = [
      {
        id: "created",
        label: "Issue created",
        detail: flow.issue.title,
        timestamp: flow.issue.createdAt,
        status: flow.issue.status,
      },
    ]
    for (const item of flow.runs) {
      events.push({
        id: `${item.run.id}-created`,
        label: "Workflow run created",
        detail: item.run.id,
        timestamp: item.run.createdAt,
        status: item.run.status,
      })
      for (const step of item.steps) {
        const timestamp = step.endedAt ?? step.startedAt ?? step.createdAt
        events.push({
          id: step.id,
          label: `Step ${step.stepId}`,
          detail: step.outputSummary || step.errorMessage || step.prompt,
          timestamp,
          status: step.status,
        })
      }
    }
    for (const task of flow.agentTasks) {
      events.push({
        id: task.id,
        label: "Agent run created",
        detail: task.title || task.summary,
        timestamp: task.createdAt,
        status: task.status,
      })
    }
    // Comments belong on the same column as runs: an issue's history is what
    // people said and what execution did, in one order.
    for (const comment of comments) {
      events.push({
        id: comment.id,
        label: commentEventLabel(comment.author_kind),
        detail: comment.body,
        timestamp: comment.created_at,
      })
    }
    return events.sort((a, b) => (a.timestamp < b.timestamp ? 1 : a.timestamp > b.timestamp ? -1 : 0))
  }, [flow, comments])

  function commentEventLabel(kind: ApiIssueComment["author_kind"]): string {
    switch (kind) {
      case "user":
        return "Comment"
      case "local_agent":
        return "Local agent report"
      default:
        return "Agent comment"
    }
  }

  function memberLabel(member: ApiTeamMember): string {
    if (member.user_id === userId) return "Me"
    if (member.user_name && member.user_name.trim() !== "") return member.user_name
    if (member.user_email && member.user_email.trim() !== "") return member.user_email
    return `Member ${member.user_id.slice(0, 8)}`
  }

  function handleSave() {
    if (!token || !currentTeamId || !flow) return
    const [kind, id] = assigneeValue ? assigneeValue.split(":") : ["", ""]
    if (kind === "workflow" && !canAssignWorkflow) {
      setError("Workflow assignment is limited to team owners and admins")
      return
    }
    setSaving(true)
    setError(null)
    updateIssue(
      currentTeamId,
      flow.issue.id,
      {
        version: flow.issue.version,
        title: title.trim(),
        description,
        status,
        assignee_kind: (kind as "person" | "agent" | "workflow" | "") || "",
        assignee_id: id || "",
      },
      token,
    )
      .then(() => load())
      .catch((err) => {
        // A conflict means someone else saved first. Reloading is what makes
        // the form usable again: the version it holds is stale, so every
        // further save would be refused for the same reason.
        if (err instanceof ApiRequestError && err.status === 409) {
          setError("This issue changed while you were editing it. It has been reloaded — reapply your change.")
          void load()
          return
        }
        setError(getErrorMessage(err, "Failed to update issue"))
      })
      .finally(() => setSaving(false))
  }

  function handleAddSubIssue() {
    if (!token || !currentTeamId || !flow) return
    const trimmed = subIssueTitle.trim()
    if (!trimmed || addingSubIssue) return
    setAddingSubIssue(true)
    setError(null)
    createIssue(currentTeamId, { title: trimmed, parent_issue_id: flow.issue.id }, token)
      .then(() => {
        setSubIssueTitle("")
        return load()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to add sub-issue")))
      .finally(() => setAddingSubIssue(false))
  }

  function handleRunWorkflow() {
    if (!token || !currentTeamId || !flow) return
    setRunningWorkflow(true)
    setError(null)
    runIssueWorkflow(currentTeamId, flow.issue.id, token)
      .then((detail) => {
        void load()
        navigate({ name: "workflowRun", workflowRunId: detail.run.id })
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to run workflow")))
      .finally(() => setRunningWorkflow(false))
  }

  function handleCancelTask(taskId: string) {
    if (!token || !currentTeamId || cancelingTaskId) return
    setCancelingTaskId(taskId)
    setError(null)
    cancelTask(currentTeamId, taskId, token)
      .then(() => load())
      .catch((err) => setError(getErrorMessage(err, "Failed to stop this run")))
      .finally(() => setCancelingTaskId(null))
  }

  function handleRetryTask(taskId: string) {
    if (!token || !currentTeamId || retryingTaskId) return
    setRetryingTaskId(taskId)
    setError(null)
    retryTask(currentTeamId, taskId, token)
      .then(() => load())
      .catch((err) => setError(getErrorMessage(err, "Failed to retry this run")))
      .finally(() => setRetryingTaskId(null))
  }

  function handleRunAgent() {
    if (!token || !currentTeamId || !flow) return
    setRunningAgent(true)
    setError(null)
    runIssueAgent(currentTeamId, flow.issue.id, token)
      .then(() => {
        void load()
      })
      .catch((err) => setError(getErrorMessage(err, "Failed to run agent")))
      .finally(() => setRunningAgent(false))
  }

  return (
    <div className="page-activity">
      <div className="page-activity__head">
        <div>
          <h1 className="page-activity__title">Issue Detail</h1>
          <p className="page-activity__subtitle">
            Inspect business state, ownership, workflow progress, and execution history in one place.
          </p>
        </div>
        <div className="page-activity__actions">
          <button type="button" className="page-activity__action-btn" onClick={() => navigate({ name: "issues" })}>
            Back to Issues
          </button>
          <button type="button" className="page-activity__action-btn" disabled={loading} onClick={() => void load()}>
            Refresh
          </button>
          {isWorkflowAssigned ? (
            <button
              type="button"
              className="page-activity__action-btn"
              disabled={runningWorkflow || loading || assignedWorkflowStatus !== "published"}
              onClick={handleRunWorkflow}
            >
              {runningWorkflow ? "Running..." : "Run Workflow"}
            </button>
          ) : null}
          {isAgentAssigned ? (
            <button
              type="button"
              className="page-activity__action-btn"
              disabled={runningAgent || loading}
              onClick={handleRunAgent}
            >
              {runningAgent ? "Running..." : "Run Agent"}
            </button>
          ) : null}
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={saving || loading || !title.trim()}
            onClick={handleSave}
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>

      {error ? <p className="page-activity__empty">{error}</p> : null}

      {loading ? (
        <p className="page-activity__empty">Loading...</p>
      ) : flow == null ? (
        <p className="page-activity__empty">Issue not found.</p>
      ) : (
        <div className="issue-detail-page__grid">
          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Issue</h2>
              <span className="issues-page__status">{flow.issue.status}</span>
            </div>
            <div className="issues-page__form">
              <label className="issues-page__field">
                <span className="issues-page__field-label">Title</span>
                <input className="issues-page__input" value={title} onChange={(e) => setTitle(e.target.value)} />
              </label>
              <label className="issues-page__field">
                <span className="issues-page__field-label">Description</span>
                <textarea
                  className="issues-page__textarea"
                  rows={8}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </label>
              <div className="issue-detail-page__split">
                <label className="issues-page__field">
                  <span className="issues-page__field-label">Business Status</span>
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
                    {canAssignWorkflow
                      ? publishedAssignableWorkflows.map((workflow) => (
                          <option key={workflow.id} value={`workflow:${workflow.id}`}>
                            {workflow.name}{workflow.status !== "published" ? ` (${workflow.status})` : ""}
                          </option>
                        ))
                      : null}
                  </select>
                  <span className="issues-page__field-label">
                    {canAssignWorkflow
                      ? "Only `published` workflows are available for new assignment."
                      : "You can still assign a person or agent here. Workflow assignment is limited to team owners and admins."}
                  </span>
                </label>
              </div>
              {status === "done" && openChildCount > 0 ? (
                <p className="page-activity__meta">
                  {openChildCount} sub-issue{openChildCount === 1 ? " is" : "s are"} still open. Closing this issue
                  anyway is allowed — sub-issue status is never rolled up.
                </p>
              ) : null}
              <div className="issues-page__meta-row">
                <div className="page-activity__meta">Current assignee: {assigneeLabel(flow.issue)}</div>
                <div className="page-activity__meta">Created: {formatTimestamp(flow.issue.createdAt)}</div>
                <div className="page-activity__meta">Updated: {formatTimestamp(flow.issue.updatedAt)}</div>
              </div>
            </div>
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">{flow.parent ? "Parent Issue" : "Sub-issues"}</h2>
              {flow.parent ? null : (
                <span className="page-activity__meta">
                  {flow.issue.childCount === 0
                    ? "None yet"
                    : `${flow.issue.doneChildCount}/${flow.issue.childCount} done`}
                </span>
              )}
            </div>
            {flow.parent ? (
              <div className="issue-detail-page__parent">
                <button
                  type="button"
                  className="page-activity__action-btn"
                  onClick={() => navigate({ name: "issue", issueId: flow.parent!.id })}
                >
                  ← {flow.parent.title}
                </button>
                <p className="page-activity__meta">
                  This is a sub-issue. Sub-issues cannot have sub-issues of their own.
                </p>
              </div>
            ) : (
              <>
                {flow.children.length === 0 ? (
                  <p className="page-activity__empty">No sub-issues yet.</p>
                ) : (
                  <ul className="issue-detail-page__children">
                    {flow.children.map((child) => (
                      <li key={child.id} className="issue-detail-page__child">
                        <button
                          type="button"
                          className="page-activity__action-btn"
                          onClick={() => navigate({ name: "issue", issueId: child.id })}
                        >
                          {child.title}
                        </button>
                        <span className="issues-page__status">{child.status}</span>
                        <span className="page-activity__meta">{assigneeLabel(child)}</span>
                      </li>
                    ))}
                  </ul>
                )}
                {/* A sub-issue starts as a title. Everything else is filled in
                    on its own page, so decomposing an issue stays one keystroke
                    per piece. */}
                <div className="issue-detail-page__child-actions">
                  <input
                    className="issues-page__input"
                    value={subIssueTitle}
                    placeholder="New sub-issue title"
                    onChange={(e) => setSubIssueTitle(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault()
                        handleAddSubIssue()
                      }
                    }}
                  />
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={handleAddSubIssue}
                    disabled={addingSubIssue || subIssueTitle.trim() === ""}
                  >
                    {addingSubIssue ? "Adding…" : "Add sub-issue"}
                  </button>
                </div>
              </>
            )}
          </section>

          <section className="issues-page__panel issue-detail-page__wide">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Discussion</h2>
              <span className="page-activity__meta">
                {comments.length === 0
                  ? "No comments"
                  : `${comments.length} comment${comments.length === 1 ? "" : "s"}`}
              </span>
            </div>
            <IssueDiscussion
              teamId={currentTeamId}
              issueId={flow.issue.id}
              token={token}
              userId={userId ?? null}
              canModerate={currentUserRole === "owner"}
              members={members}
              agentNames={agentNames}
              onOpenTrace={(taskRunId) => setTraceRunId(taskRunId)}
              onCommentsChanged={setComments}
            />
          </section>

          <section className="issues-page__panel issue-detail-page__wide">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Results</h2>
              <span className="page-activity__meta">
                {flow.outputs.length === 0
                  ? "No outputs yet"
                  : `${flow.outputs.length} output${flow.outputs.length === 1 ? "" : "s"}`}
              </span>
            </div>
            <OutputsList
              outputs={flow.outputs}
              token={token}
              onOpenFull={(o) => setViewerOutput(o)}
              onOpenConversation={(conversationId) => navigate({ name: "conversation", conversationId })}
              onOpenRun={(workflowRunId) => navigate({ name: "workflowRun", workflowRunId })}
              onOpenTrace={(taskRunId) => setTraceRunId(taskRunId)}
            />
          </section>

          {isWorkflowAssigned && assignedWorkflowStatus !== "published" ? (
            <section className="issues-page__panel">
              <div className="issues-page__toolbar">
                <h2 className="issues-page__section-title">Workflow Availability</h2>
              </div>
              <p className="page-activity__empty">
                The assigned workflow is currently `{assignedWorkflowStatus ?? "unknown"}` and cannot be run until it is `published`.
              </p>
            </section>
          ) : null}

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Execution Summary</h2>
              <span className="issues-page__status">{currentRun?.run.status ?? latestAgentTask?.status ?? "no_runs"}</span>
            </div>
            {currentRun ? (
              <div className="workflow-run-page__meta">
                <div><strong>Latest run:</strong> {currentRun.run.id}</div>
                <div><strong>Workflow:</strong> {flow.workflow?.name ?? currentRun.run.workflowId}</div>
                <div><strong>Started:</strong> {currentRun.run.startedAt ? formatTimestamp(currentRun.run.startedAt) : "Not started"}</div>
                <div><strong>Steps:</strong> {currentRun.steps.filter((step) => step.status === "succeeded").length} / {currentRun.steps.length} done</div>
                {currentRun.run.errorMessage ? <div className="modal__error">{currentRun.run.errorMessage}</div> : null}
                <div className="workflow-run-page__step-actions">
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => navigate({ name: "workflowRun", workflowRunId: currentRun.run.id })}
                  >
                    Open Run Detail
                  </button>
                  {currentRunLatestTaskId ? (
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      onClick={() => navigate({ name: "task", taskId: currentRunLatestTaskId })}
                    >
                      Open Task
                    </button>
                  ) : null}
                </div>
              </div>
            ) : latestAgentTask ? (
              <div className="workflow-run-page__meta">
                <div><strong>Latest agent task:</strong> {latestAgentTask.id}</div>
                <div><strong>Agent:</strong> {assigneeLabel(flow.issue)}</div>
                <div><strong>Created:</strong> {formatTimestamp(latestAgentTask.createdAt)}</div>
                <div><strong>Status:</strong> {latestAgentTask.status}</div>
                <div className="workflow-run-page__step-actions">
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    onClick={() => navigate({ name: "task", taskId: latestAgentTask.id })}
                  >
                    Open Task
                  </button>
                  {taskIsStoppable(latestAgentTask.status) ? (
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      disabled={cancelingTaskId === latestAgentTask.id}
                      onClick={() => handleCancelTask(latestAgentTask.id)}
                    >
                      {cancelingTaskId === latestAgentTask.id ? "Stopping..." : "Stop Run"}
                    </button>
                  ) : null}
                  {taskIsRetryable(latestAgentTask.status) ? (
                    <button
                      type="button"
                      className="page-activity__action-btn"
                      disabled={retryingTaskId === latestAgentTask.id}
                      onClick={() => handleRetryTask(latestAgentTask.id)}
                    >
                      {retryingTaskId === latestAgentTask.id ? "Retrying..." : "Retry Run"}
                    </button>
                  ) : null}
                </div>
              </div>
            ) : (
              <p className="page-activity__empty">No execution runs recorded for this issue yet.</p>
            )}
          </section>

          <section className="issues-page__panel issue-detail-page__wide">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Flow Steps</h2>
              <span className="page-activity__meta">{currentRun?.steps.length ?? 0} latest-run steps</span>
            </div>
            {currentRun && currentRun.steps.length > 0 ? (
              <ol className="workflow-page__steps">
                {currentRun.steps.map((step) => (
                  <li key={step.id} className="workflow-page__step">
                    <div className="workflow-page__step-head">
                      <strong>{step.stepId}</strong>
                      <span className="issues-page__status">{step.status}</span>
                    </div>
                    <div className="workflow-page__step-body">
                      <div className="page-activity__meta">{step.stepType}</div>
                      <div>{step.prompt}</div>
                      {step.outputSummary ? <pre className="workflow-page__step-output">{step.outputSummary}</pre> : null}
                      {step.errorMessage ? <p className="modal__error">{step.errorMessage}</p> : null}
                    </div>
                  </li>
                ))}
              </ol>
            ) : (
              <p className="page-activity__empty">No step state available.</p>
            )}
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Timeline</h2>
              <span className="page-activity__meta">{timeline.length} events</span>
            </div>
            <ol className="issue-detail-page__timeline">
              {timeline.map((event) => (
                <li key={event.id} className="issue-detail-page__timeline-item">
                  <div>
                    <strong>{event.label}</strong>
                    <div className="page-activity__meta">{formatTimestamp(event.timestamp)}</div>
                  </div>
                  <div className="issue-detail-page__timeline-detail">
                    {event.status ? <span className="issues-page__status">{event.status}</span> : null}
                    <span>{event.detail}</span>
                  </div>
                </li>
              ))}
            </ol>
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Run History</h2>
              <span className="page-activity__meta">{flow.total} total</span>
            </div>
            {flow.runs.length === 0 ? (
              <p className="page-activity__empty">No runs yet.</p>
            ) : (
              <ul className="workflow-page__runs">
                {flow.runs.map((item) => (
                  <li key={item.run.id}>
                    <button
                      type="button"
                      className="workflow-page__run-row"
                      onClick={() => navigate({ name: "workflowRun", workflowRunId: item.run.id })}
                    >
                      <span>
                        <strong>{item.run.id}</strong>
                        <span className="page-activity__meta workflow-detail-page__run-id">
                          {item.run.createdLabel}
                        </span>
                      </span>
                      <span className="issues-page__status">{item.run.status}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="issues-page__panel">
            <div className="issues-page__toolbar">
              <h2 className="issues-page__section-title">Agent Run Sequence</h2>
              <span className="page-activity__meta">{flow.agentTasks.length} tasks</span>
            </div>
            {flow.agentTasks.length === 0 ? (
              <p className="page-activity__empty">No agent runs recorded for this issue yet.</p>
            ) : (
              <ul className="workflow-page__runs">
                {flow.agentTasks.map((task) => (
                  <li key={task.id}>
                    <button
                      type="button"
                      className="workflow-page__run-row"
                      onClick={() => navigate({ name: "task", taskId: task.id })}
                    >
                      <span>
                        <strong>{task.title}</strong>
                        <span className="page-activity__meta workflow-detail-page__run-id">
                          {task.timeLabel}
                        </span>
                      </span>
                      <span className="issues-page__status">{task.status}</span>
                    </button>
                    {taskIsStoppable(task.status) ? (
                      <button
                        type="button"
                        className="page-activity__action-btn"
                        disabled={cancelingTaskId === task.id}
                        onClick={() => handleCancelTask(task.id)}
                      >
                        {cancelingTaskId === task.id ? "Stopping..." : "Stop Run"}
                      </button>
                    ) : null}
                    {taskIsRetryable(task.status) ? (
                      <button
                        type="button"
                        className="page-activity__action-btn"
                        disabled={retryingTaskId === task.id}
                        onClick={() => handleRetryTask(task.id)}
                      >
                        {retryingTaskId === task.id ? "Retrying..." : "Retry Run"}
                      </button>
                    ) : null}
                    <pre className="workflow-page__step-output">{task.summary}</pre>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      )}
      <OutputViewerModal
        open={viewerOutput != null}
        teamId={currentTeamId}
        token={token}
        output={viewerOutput}
        onClose={() => setViewerOutput(null)}
      />
      <RunTraceModal
        open={traceRunId != null}
        teamId={currentTeamId}
        token={token}
        taskRunId={traceRunId}
        onClose={() => setTraceRunId(null)}
      />
    </div>
  )
}
