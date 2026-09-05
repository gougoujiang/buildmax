import { useCallback, useEffect, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer } from "@buildmax/gui"
import { AgentAvatar } from "../../components/UserAvatar"
import { useApp } from "../../contexts/AppContext"
import { useTeam } from "../../contexts/TeamContext"
import { cancelTask, continueTask, getTask, getTaskRuns, retryTask } from "../../features/tasks"
import { getAgent } from "../../features/agents"
import { RunTraceModal } from "../../features/runs"
import { TaskFilesModal } from "../../features/conversations"
import { runStatusLabel, runStatusTone } from "../../features/conversations/thread"
import { navigate } from "../../router"
import type { ApiTask, ApiTaskRun } from "../../lib/api/types"
import type { BreadcrumbCrumb } from "../../lib/types"
import { getErrorMessage } from "../../lib/errorMessage"

interface TaskDetailProps {
  token: string | null
  taskId: string
}

const activeStatuses = new Set(["PENDING", "SCHEDULED", "RUNNING"])

function fmtDuration(start?: string | null, end?: string | null): string {
  if (!start || !end) return "—"
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (!Number.isFinite(ms) || ms < 0) return "—"
  const s = Math.round(ms / 1000)
  const m = Math.floor(s / 60)
  return m > 0 ? `${m}m ${s % 60}s` : `${s}s`
}

function fmtWhen(ts?: string | null): string {
  if (!ts) return "—"
  const d = new Date(ts)
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString()
}

function StatusPill({ status }: { status: string }) {
  return (
    <span className={`run-pill run-pill--${runStatusTone(status)}`}>{runStatusLabel(status)}</span>
  )
}

export function TaskDetail({ token, taskId }: TaskDetailProps) {
  const { currentTeamId } = useTeam()
  const { entityLabels, setEntityLabel, setBreadcrumbTrail } = useApp()
  const [task, setTask] = useState<ApiTask | null>(null)
  const [runs, setRuns] = useState<ApiTaskRun[]>([])
  const [agentName, setAgentName] = useState<string | null>(null)
  const [input, setInput] = useState("")
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [traceRunId, setTraceRunId] = useState<string | null>(null)
  const [filesRunId, setFilesRunId] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!token || !currentTeamId) return
    try {
      const [nextTask, nextRuns] = await Promise.all([
        getTask(currentTeamId, taskId, token),
        getTaskRuns(currentTeamId, taskId, token),
      ])
      setTask(nextTask)
      setRuns(nextRuns)
      setError(null)
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load task"))
    } finally {
      setLoading(false)
    }
  }, [currentTeamId, taskId, token])

  useEffect(() => {
    setLoading(true)
    void load()
  }, [load])

  const running = runs.some((run) => activeStatuses.has(run.status))
  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => void load(), 1500)
    return () => window.clearInterval(timer)
  }, [load, running])

  // Resolve the agent's name for the header, the "Open agent" link, and the
  // info panel, and share it so this task's breadcrumb reads the name too.
  useEffect(() => {
    if (!token || !currentTeamId || !task?.agent_id) {
      setAgentName(null)
      return
    }
    getAgent(currentTeamId, task.agent_id, token)
      .then((a) => setAgentName(a.name))
      .catch(() => setAgentName(null))
  }, [token, currentTeamId, task?.agent_id])

  useEffect(() => {
    if (task?.agent_id && agentName) setEntityLabel(task.agent_id, agentName)
  }, [task?.agent_id, agentName, setEntityLabel])

  // Origin-aware breadcrumb: back to the agent, issue, or conversation.
  useEffect(() => {
    if (!task) return
    const leaf: BreadcrumbCrumb = { label: task.title || "Task", route: { name: "task", taskId } }
    let trail: BreadcrumbCrumb[]
    if (task.agent_id) {
      trail = [
        { label: "Agents", route: { name: "agents" } },
        { label: entityLabels[task.agent_id] ?? "Agent", route: { name: "agent", agentId: task.agent_id } },
        leaf,
      ]
    } else if (task.issue_id) {
      trail = [
        { label: "Issues", route: { name: "issues" } },
        { label: entityLabels[task.issue_id] ?? "Issue", route: { name: "issue", issueId: task.issue_id } },
        leaf,
      ]
    } else if (task.conversation_id) {
      trail = [
        { label: "Home", route: { name: "home" } },
        { label: "Conversation", route: { name: "conversation", conversationId: task.conversation_id } },
        leaf,
      ]
    } else {
      trail = [{ label: "Home", route: { name: "home" } }, leaf]
    }
    setBreadcrumbTrail(taskId, trail)
  }, [task, taskId, entityLabels, setBreadcrumbTrail])

  async function handleContinue() {
    const message = input.trim()
    if (!message || !token || !currentTeamId || sending || running) return
    setSending(true)
    setError(null)
    try {
      // Generated fresh per attempt: this call's own retry-on-401 reuses it, so
      // a token refresh cannot turn one Continue into two runs.
      const run = await continueTask(currentTeamId, taskId, message, token, crypto.randomUUID())
      setRuns((current) => [...current, run])
      setInput("")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to continue task"))
    } finally {
      setSending(false)
    }
  }

  function handleStop() {
    if (!token || !currentTeamId || stopping || !running) return
    setStopping(true)
    setError(null)
    cancelTask(currentTeamId, taskId, token)
      .then(() => load())
      .catch((err) => setError(getErrorMessage(err, "Failed to stop this run")))
      .finally(() => setStopping(false))
  }

  function handleRetry() {
    if (!token || !currentTeamId || retrying || running) return
    setRetrying(true)
    setError(null)
    retryTask(currentTeamId, taskId, token)
      .then(() => load())
      .catch((err) => setError(getErrorMessage(err, "Failed to retry this run")))
      .finally(() => setRetrying(false))
  }

  function runIndexOf(runId: string | null | undefined): number {
    if (!runId) return -1
    return runs.findIndex((r) => r.id === runId)
  }

  const hasFiles = (runId: string) => task?.artifact_run_ids?.includes(runId) ?? false

  return (
    <div className="task-detail">
      <header className="task-detail__header">
        <div className="task-detail__ident">
          <AgentAvatar size="md" className="task-detail__avatar" />
          <div>
            <h1 className="page-activity__title">{task?.title || "Task"}</h1>
            <div className="task-detail__sub">
              {task ? <StatusPill status={task.status} /> : <span>loading…</span>}
              {runs[0]?.trigger_source ? <span>· {runs[0].trigger_source}</span> : null}
              {task?.started_at ? <span>· started {fmtWhen(task.started_at)}</span> : null}
              {task?.started_at && task?.ended_at ? (
                <span>· {fmtDuration(task.started_at, task.ended_at)}</span>
              ) : null}
            </div>
          </div>
        </div>
        <div className="task-detail__actions">
          {running ? (
            <button type="button" className="page-activity__action-btn" disabled={stopping} onClick={handleStop}>
              {stopping ? "Stopping…" : "Stop"}
            </button>
          ) : runs.length > 0 ? (
            <button type="button" className="page-activity__action-btn" disabled={retrying} onClick={handleRetry}>
              {retrying ? "Retrying…" : "Retry last run"}
            </button>
          ) : null}
          {task?.agent_id ? (
            <button
              type="button"
              className="page-activity__action-btn"
              onClick={() => navigate({ name: "agent", agentId: task.agent_id! })}
            >
              Open agent
            </button>
          ) : null}
        </div>
      </header>

      <div className="task-detail__body">
        <div className="task-detail__main">
          {loading ? (
            <p className="page-activity__empty">Loading…</p>
          ) : error ? (
            <p className="page-activity__empty">{error}</p>
          ) : runs.length === 0 ? (
            <p className="page-activity__empty">No runs yet.</p>
          ) : (
            <div className="run-cards">
              {runs.map((run, i) => {
                const retryIdx = runIndexOf(run.retry_of_task_run_id)
                return (
                  <article key={run.id} className="run-card">
                    <header className="run-card__head">
                      <span className="run-card__n">Run {i + 1}</span>
                      <StatusPill status={run.status} />
                      {run.trigger_source ? <span className="run-card__meta">{run.trigger_source}</span> : null}
                      <span className="run-card__meta">{fmtDuration(run.started_at, run.ended_at)}</span>
                      {run.agent_revision != null ? (
                        <span className="run-card__meta">rev {run.agent_revision}</span>
                      ) : null}
                      {run.retry_of_task_run_id ? (
                        <span className="run-card__meta">
                          retry of {retryIdx >= 0 ? `run ${retryIdx + 1}` : "an earlier run"}
                        </span>
                      ) : null}
                      <div className="run-card__actions">
                        <button
                          type="button"
                          className="page-activity__action-btn page-activity__action-btn--sm"
                          onClick={() => setTraceRunId(run.id)}
                        >
                          Trace
                        </button>
                        {hasFiles(run.id) ? (
                          <button
                            type="button"
                            className="page-activity__action-btn page-activity__action-btn--sm"
                            onClick={() => setFilesRunId(run.id)}
                          >
                            Files
                          </button>
                        ) : null}
                      </div>
                    </header>
                    <div className="run-card__io">
                      <span className="run-card__label">Input</span>
                      <div className="page-chat__markdown">
                        <Markdown remarkPlugins={[remarkGfm]}>{run.input}</Markdown>
                      </div>
                      <span className="run-card__label">Output</span>
                      {run.output ? (
                        <div className="page-chat__markdown">
                          <Markdown remarkPlugins={[remarkGfm]}>{run.output}</Markdown>
                        </div>
                      ) : run.error_message ? (
                        <p className="run-card__error">{run.error_message}</p>
                      ) : (
                        <p className="run-card__pending">
                          {activeStatuses.has(run.status)
                            ? `Run ${run.status.toLowerCase()}…`
                            : `Run ${run.status.toLowerCase()}`}
                        </p>
                      )}
                    </div>
                  </article>
                )
              })}
            </div>
          )}

          <section className="task-detail__composer" aria-label="Continue task">
            <ChatComposer
              value={input}
              onChange={setInput}
              onSubmit={handleContinue}
              loading={sending}
              disabled={running || !task?.agent_id}
              error={error}
              placeholder={running ? "Wait for the current run to finish…" : "Continue this task…"}
              ariaLabel="Continue task"
              submitLabel="Continue"
            />
          </section>
        </div>

        <aside className="task-detail__info" aria-label="Run details">
          <h2 className="task-detail__info-title">Run details</h2>
          <dl className="task-kv">
            <dt>Agent</dt>
            <dd>
              {task?.agent_id ? (
                <button
                  type="button"
                  className="task-kv__link"
                  onClick={() => navigate({ name: "agent", agentId: task.agent_id! })}
                >
                  {agentName ?? "Agent"}
                </button>
              ) : (
                "—"
              )}
            </dd>
            <dt>Status</dt>
            <dd>{task ? <StatusPill status={task.status} /> : "—"}</dd>
            <dt>Trigger</dt>
            <dd>{runs[0]?.trigger_source ?? "—"}</dd>
            <dt>Started</dt>
            <dd>{fmtWhen(task?.started_at)}</dd>
            <dt>Ended</dt>
            <dd>{fmtWhen(task?.ended_at)}</dd>
            <dt>Duration</dt>
            <dd>{fmtDuration(task?.started_at, task?.ended_at)}</dd>
            <dt>Runs</dt>
            <dd>{runs.length}</dd>
            <dt>Task</dt>
            <dd className="task-kv__mono">{task?.id ?? taskId}</dd>
            {task?.issue_id ? (
              <>
                <dt>Issue</dt>
                <dd>
                  <button
                    type="button"
                    className="task-kv__link"
                    onClick={() => navigate({ name: "issue", issueId: task.issue_id! })}
                  >
                    Open issue
                  </button>
                </dd>
              </>
            ) : null}
            {task?.conversation_id ? (
              <>
                <dt>Conversation</dt>
                <dd>
                  <button
                    type="button"
                    className="task-kv__link"
                    onClick={() => navigate({ name: "conversation", conversationId: task.conversation_id! })}
                  >
                    Open conversation
                  </button>
                </dd>
              </>
            ) : null}
          </dl>
        </aside>
      </div>

      <RunTraceModal
        open={traceRunId != null}
        teamId={currentTeamId}
        token={token}
        taskRunId={traceRunId}
        onClose={() => setTraceRunId(null)}
      />
      <TaskFilesModal
        open={filesRunId != null}
        teamId={currentTeamId}
        token={token}
        taskRunId={filesRunId}
        onClose={() => setFilesRunId(null)}
      />
    </div>
  )
}
