import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer, ChatThread, type ChatThreadItem } from "@buildmax/gui"
import { AgentAvatar, UserAvatar } from "../../components/UserAvatar"
import { useAuth } from "../../contexts/AuthContext"
import { useTeam } from "../../contexts/TeamContext"
import { continueTask, getTask, getTaskRuns } from "../../features/tasks"
import type { ApiTask, ApiTaskRun } from "../../lib/api/types"
import { getErrorMessage } from "../../lib/errorMessage"

interface TaskDetailProps {
  token: string | null
  taskId: string
}

const activeStatuses = new Set(["PENDING", "SCHEDULED", "RUNNING"])

export function TaskDetail({ token, taskId }: TaskDetailProps) {
  const { currentTeamId } = useTeam()
  const { user } = useAuth()
  const historyRef = useRef<HTMLElement | null>(null)
  const [task, setTask] = useState<ApiTask | null>(null)
  const [runs, setRuns] = useState<ApiTaskRun[]>([])
  const [input, setInput] = useState("")
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
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
      const run = await continueTask(currentTeamId, taskId, message, token)
      setRuns((current) => [...current, run])
      setInput("")
    } catch (err) {
      setError(getErrorMessage(err, "Failed to continue task"))
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="page-chat task-thread">
      <header className="task-thread__header">
        <div>
          <h1 className="page-activity__title">{task?.title || "Task"}</h1>
          <p className="page-activity__subtitle">Agent execution history · {task?.status?.toLowerCase() || "loading"}</p>
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
