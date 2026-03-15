import { useEffect, useRef, useState } from "react"
import { getErrorMessage } from "../../../lib/errorMessage"
import { useFetch } from "../../../hooks/useFetch"
import {
  createTaskRun,
  getTaskConversation,
  subscribeTaskStream,
} from "../api"

interface UseTaskDetailOptions {
  profileId: string
  taskId: string
  token: string | null
  initialInput?: string
  onRunComplete?: () => void
}

export function useTaskDetail({
  profileId,
  taskId,
  token,
  initialInput,
  onRunComplete,
}: UseTaskDetailOptions) {
  const streamCleanupRef = useRef<(() => void) | null>(null)
  const historyRef = useRef<HTMLElement | null>(null)

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

  function toggleToolExpand(index: number) {
    setExpandedToolIndices((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  useEffect(() => {
    return () => {
      if (streamCleanupRef.current) {
        streamCleanupRef.current()
        streamCleanupRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    if (!token || !profileId || !taskId) return
    const cleanup = subscribeTaskStream(profileId, taskId, token, {
      onDelta: (text) => setStreamingContent((prev) => prev + text),
      onDone: () => {
        setStreamingContent("")
        setLastSentMessage(null)
        setFollowUpLoading(false)
        refetchSession()
        onRunComplete?.()
      },
      onError: (err) => {
        setFollowUpError(getErrorMessage(err, "Stream failed"))
        setFollowUpLoading(false)
      },
    })
    streamCleanupRef.current = cleanup
    return () => {
      cleanup()
      streamCleanupRef.current = null
    }
  }, [profileId, taskId, token, refetchSession, onRunComplete])

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
