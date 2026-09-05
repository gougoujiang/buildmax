import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer, ChatThread, type ChatThreadItem } from "@buildmax/gui"
import { AgentAvatar, UserAvatar } from "../../components/UserAvatar"
import { useApp } from "../../contexts/AppContext"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"
import { cancelTask, continueTask, getTask, getTaskRuns, retryTask } from "../../features/tasks"
import type { ApiTask, ApiTaskRun } from "../../lib/api/types"
import type { BreadcrumbCrumb } from "../../lib/types"
import { getErrorMessage } from "../../lib/errorMessage"

interface TaskDetailProps {
  token: string | null
  taskId: string
}

const activeStatuses = new Set(["PENDING", "SCHEDULED", "RUNNING"])

export function TaskDetail({ token, taskId }: TaskDetailProps) {
  const { currentTeamId } = useTeam()
  const { user } = useAuth()
  const { entityLabels, setBreadcrumbTrail } = useApp()
  const historyRef = useRef<HTMLElement | null>(null)
  const [task, setTask] = useState<ApiTask | null>(null)
  const [runs, setRuns] = useState<ApiTaskRun[]>([])
  const [input, setInput] = useState("")
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [error, setError] = useState<string | null>(null)

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

  // Publish an origin-aware breadcrumb trail so the shell shows the way back to
  // wherever this task belongs, not just "Home". The agent/issue label is
  // whatever that detail page has already registered, else a generic fallback.
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

  const running = runs.some((run) => activeStatuses.has(run.status))
  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => void load(), 1500)
    return () => window.clearInterval(timer)
  }, [load, running])

  useEffect(() => {
    historyRef.current?.scrollTo({ top: historyRef.current.scrollHeight, behavior: "smooth" })
  }, [runs])

  const items = useMemo<ChatThreadItem[]>(() => {
    return runs.flatMap((run) => {
      const turns: ChatThreadItem[] = [{
        id: `${run.id}-input`,
        role: "user",
        label: "You",
        avatar: user ? <UserAvatar user={user} size="sm" /> : undefined,
        body: <div className="page-chat__msg-content page-chat__markdown"><Markdown remarkPlugins={[remarkGfm]}>{run.input}</Markdown></div>,
      }]
      const status = run.status.toLowerCase()
      if (run.output) {
        turns.push({
          id: `${run.id}-output`, role: "assistant", label: `Agent · ${status}`,
          avatar: <AgentAvatar size="sm" />,
          body: <div className="page-chat__msg-content page-chat__markdown"><Markdown remarkPlugins={[remarkGfm]}>{run.output}</Markdown></div>,
        })
      } else {
        turns.push({
          id: `${run.id}-status`, role: "assistant", label: `Agent · ${status}`,
          avatar: <AgentAvatar size="sm" />,
          body: <p className="bm-chat-thread__text bm-chat-thread__text--muted">{run.error_message || (activeStatuses.has(run.status) ? `Run ${status}…` : `Run ${status}`)}</p>,
        })
      }
      return turns
    })
  }, [runs, user])

  async function handleContinue() {
    const message = input.trim()
    if (!message || !token || !currentTeamId || sending || running) return
    setSending(true)
    setError(null)
    try {
      // Generated fresh per attempt: this call's own retry-on-401 reuses it, so
      // a token refresh cannot turn one Continue into two runs. It is not
      // meant to survive a page reload or a second manual click.
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
          <p className="page-activity__subtitle">Agent execution history · {task?.status?.toLowerCase() || "loading"}</p>
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
        </div>
      </header>
      <ChatThread
        historyRef={historyRef}
        ariaLabel="Agent task history"
        items={items}
        loadingText={loading ? "Loading task history…" : null}
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
    </div>
  )
}
