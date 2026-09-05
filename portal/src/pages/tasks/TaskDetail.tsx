import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer, ChatThread, type ChatThreadItem } from "@buildmax/gui"
import { AgentAvatar, UserAvatar } from "../../components/UserAvatar"
import { useApp } from "../../contexts/AppContext"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"
import { cancelTask, continueTask, getTask, getTaskRuns, retryTask } from "../../features/tasks"
import { getAgent } from "../../features/agents"
import { RunTraceModal } from "../../features/runs"
import { TaskFilesModal } from "../../features/conversations"
import { navigate } from "../../router"
import type { ApiTask, ApiTaskRun } from "../../lib/api/types"
import type { BreadcrumbCrumb } from "../../lib/types"
import { getErrorMessage } from "../../lib/errorMessage"

interface TaskDetailProps {
  token: string | null
  taskId: string
}

const activeStatuses = new Set(["PENDING", "SCHEDULED", "RUNNING"])

function fmtDuration(start?: string | null, end?: string | null): string | null {
  if (!start || !end) return null
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (!Number.isFinite(ms) || ms < 0) return null
  const s = Math.round(ms / 1000)
  const m = Math.floor(s / 60)
  return m > 0 ? `${m}m ${s % 60}s` : `${s}s`
}

export function TaskDetail({ token, taskId }: TaskDetailProps) {
  const { currentTeamId } = useTeam()
  const { user } = useAuth()
  const { entityLabels, setEntityLabel, setBreadcrumbTrail } = useApp()
  const historyRef = useRef<HTMLElement | null>(null)
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

  useEffect(() => {
    historyRef.current?.scrollTo({ top: historyRef.current.scrollHeight, behavior: "smooth" })
  }, [runs])

  // Resolve the agent's name for the header link, and share it so this task's
  // breadcrumb reads the name too.
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

  const hasFiles = useCallback(
    (runId: string) => task?.artifact_run_ids?.includes(runId) ?? false,
    [task],
  )

  // The task rendered as a conversation: each run is one user turn (its input)
  // and one agent turn (its output). The run's technical detail — status,
  // timing, trace, files — is secondary, tucked into a small footer on the
  // agent turn rather than a heading, so the conversation stays the subject.
  const items = useMemo<ChatThreadItem[]>(() => {
    return runs.flatMap((run) => {
      const status = run.status.toLowerCase()
      const duration = fmtDuration(run.started_at, run.ended_at)
      const turns: ChatThreadItem[] = [
        {
          id: `${run.id}-input`,
          role: "user",
          label: "You",
          avatar: user ? <UserAvatar user={user} size="sm" /> : undefined,
          body: (
            <div className="page-chat__msg-content page-chat__markdown">
              <Markdown remarkPlugins={[remarkGfm]}>{run.input}</Markdown>
            </div>
          ),
        },
      ]
      const detail = (
        <div className="run-turn__foot">
          {duration ? <span className="run-turn__meta">{duration}</span> : null}
          <button type="button" className="run-turn__link" onClick={() => setTraceRunId(run.id)}>
            Details
          </button>
          {hasFiles(run.id) ? (
            <button type="button" className="run-turn__link" onClick={() => setFilesRunId(run.id)}>
              Files
            </button>
          ) : null}
        </div>
      )
      turns.push({
        id: `${run.id}-output`,
        role: "assistant",
        label: `Agent · ${status}`,
        avatar: <AgentAvatar size="sm" />,
        body: (
          <div>
            {run.output ? (
              <div className="page-chat__msg-content page-chat__markdown">
                <Markdown remarkPlugins={[remarkGfm]}>{run.output}</Markdown>
              </div>
            ) : (
              <p className="bm-chat-thread__text bm-chat-thread__text--muted">
                {run.error_message || (activeStatuses.has(run.status) ? `Run ${status}…` : `Run ${status}`)}
              </p>
            )}
            {detail}
          </div>
        ),
      })
      return turns
    })
  }, [runs, user, hasFiles])

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

  return (
    <div className="page-chat task-thread">
      <header className="task-thread__header">
        <div>
          <h1 className="page-activity__title">{task?.title || "Task"}</h1>
          <p className="page-activity__subtitle">
            {agentName ? `${agentName} · ` : ""}
            {task?.status?.toLowerCase() || "loading"}
          </p>
        </div>
        <div className="task-thread__header-actions">
          {running ? (
            <button type="button" className="page-activity__action-btn" disabled={stopping} onClick={handleStop}>
              {stopping ? "Stopping..." : "Stop"}
            </button>
          ) : runs.length > 0 ? (
            <button type="button" className="page-activity__action-btn" disabled={retrying} onClick={handleRetry}>
              {retrying ? "Retrying..." : "Retry last run"}
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
      <ChatThread
        historyRef={historyRef}
        ariaLabel="Task conversation"
        items={items}
        loadingText={loading ? "Loading conversation…" : null}
        errorText={error}
        emptyText="No runs yet."
      />
      <section className="page-chat__input" aria-label="Continue task">
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
