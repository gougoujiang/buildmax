import { useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../../../lib/errorMessage"
import { useFetch } from "../../../hooks/useFetch"
import { createTaskRun, getTaskConversation } from "../api"
import { useWebSocket } from "../../../contexts/WebSocketContext"

interface UseTaskDetailOptions {
  profileId: string
  taskId: string
  token: string | null
  initialInput?: string
  onRunComplete?: () => void
}

interface TaskStreamDeltaPayload {
  task_id: string
  delta: string
}

interface TaskStreamDonePayload {
  task_id: string
}

export function useTaskDetail({
  profileId,
  taskId,
  token,
  initialInput,
  onRunComplete,
}: UseTaskDetailOptions) {
  const historyRef = useRef<HTMLElement | null>(null)
  const ws = useWebSocket()

  const {
    data: session,
    loading: sessionLoading,
    error: sessionError,
    refetch: refetchSession,
  } = useFetch(
    () => getTaskConversation(profileId, taskId, token!),
    [profileId, taskId, token],
    {
      enabled: !!(token && profileId && taskId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load session"),
    }
  )

  const [followUpInput, setFollowUpInput] = useState("")
  const [followUpLoading, setFollowUpLoading] = useState(false)
  const [followUpError, setFollowUpError] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState("")
  const [lastSentMessage, setLastSentMessage] = useState<string | null>(null)
  const [expandedToolIndices, setExpandedToolIndices] = useState<Set<number>>(new Set())

  const refetchSessionRef = useRef(refetchSession)
  refetchSessionRef.current = refetchSession
  const onRunCompleteRef = useRef(onRunComplete)
  onRunCompleteRef.current = onRunComplete

  function toggleToolExpand(index: number) {
    setExpandedToolIndices((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  // Subscribe to task stream via WebSocket
  useEffect(() => {
    if (!token || !taskId) return

    const handleDelta = (payload: TaskStreamDeltaPayload) => {
      if (payload.task_id !== taskId) return
      setStreamingContent((prev) => prev + payload.delta)
    }

    const handleDone = (payload: TaskStreamDonePayload) => {
      if (payload.task_id !== taskId) return
      setStreamingContent("")
      setLastSentMessage(null)
      setFollowUpLoading(false)
      refetchSessionRef.current()
      onRunCompleteRef.current?.()
    }

    ws.on("task.stream.delta", handleDelta)
    ws.on("task.stream.done", handleDone)
    ws.subscribeTask(taskId)

    return () => {
      ws.off("task.stream.delta", handleDelta)
      ws.off("task.stream.done", handleDone)
      ws.unsubscribeTask(taskId)
    }
  }, [ws, taskId, token])

  useEffect(() => {
    if (!initialInput || session !== null || sessionLoading || sessionError) return
    const interval = setInterval(() => refetchSession(), 2500)
    return () => clearInterval(interval)
  }, [initialInput, session, sessionLoading, sessionError, refetchSession])

  useEffect(() => {
    const el = historyRef.current
    if (!el) return
    const id = requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight
    })
    return () => cancelAnimationFrame(id)
  }, [session, streamingContent, lastSentMessage])

  async function submitFollowUp() {
    const input = followUpInput.trim()
    if (!input || !token || followUpLoading) return
    setFollowUpError(null)
    setFollowUpLoading(true)
    setStreamingContent("")
    setLastSentMessage(input)
    setFollowUpInput("")
    try {
      await createTaskRun(profileId, taskId, { input }, token)
    } catch (err) {
      setFollowUpError(getErrorMessage(err, "Failed to start run"))
      setFollowUpLoading(false)
    }
  }

  const showInitialInput = Boolean(
    initialInput &&
      !sessionLoading &&
      !sessionError &&
      (!session || session.messages.length === 0)
  )

  return {
    historyRef,
    session,
    sessionLoading,
    sessionError,
    followUpInput,
    setFollowUpInput,
    followUpLoading,
    followUpError,
    streamingContent,
    lastSentMessage,
    expandedToolIndices,
    toggleToolExpand,
    submitFollowUp,
    showInitialInput,
  }
}
