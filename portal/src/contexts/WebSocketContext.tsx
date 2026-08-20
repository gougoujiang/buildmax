import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { BuildMaxWebSocket } from "../lib/api/ws"
import { useAuth } from "./AuthContext"
import { useTeam } from "./TeamContext"

interface WebSocketContextValue {
  ws: BuildMaxWebSocket
  /** Mark a conversation as having a turn in flight (called when the user sends). */
  markConversationBusy: (conversationId: string) => void
  /** Read busy state for a specific conversation. */
  isConversationBusy: (conversationId: string) => boolean
  /** Set of busy conversation IDs; included so consumers can subscribe via React state. */
  busyConversations: ReadonlySet<string>
  /**
   * Messages accepted while a turn was running, per conversation, oldest first.
   * The server owns the real queue; this mirrors its queued/dequeued events so the
   * thread can show what is still waiting.
   */
  queuedMessages: ReadonlyMap<string, string[]>
}

interface QueuedPayload {
  conversation_id?: string
  content?: string
}

interface CompletedPayload {
  conversation_id?: string
  queued_remaining?: number
}

interface ErrorPayload {
  conversation_id?: string
  code?: string
}

const WebSocketContext = createContext<WebSocketContextValue | null>(null)

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const { currentTeamId } = useTeam()
  const wsRef = useRef<BuildMaxWebSocket | null>(null)

  if (!wsRef.current) {
    wsRef.current = new BuildMaxWebSocket()
  }

  const [busyConversations, setBusyConversations] = useState<Set<string>>(
    () => new Set()
  )
  const [queuedMessages, setQueuedMessages] = useState<Map<string, string[]>>(
    () => new Map()
  )

  const enqueueMessage = useCallback((conversationId: string, content: string) => {
    setQueuedMessages((prev) => {
      const next = new Map(prev)
      next.set(conversationId, [...(prev.get(conversationId) ?? []), content])
      return next
    })
  }, [])

  // Drop the first copy of content: a user who sends the same text twice has two
  // turns queued, and only the one that just started should leave the list.
  const dequeueMessage = useCallback((conversationId: string, content: string) => {
    setQueuedMessages((prev) => {
      const list = prev.get(conversationId)
      if (!list?.length) return prev
      const at = list.indexOf(content)
      const rest = at === -1 ? list.slice(1) : [...list.slice(0, at), ...list.slice(at + 1)]
      const next = new Map(prev)
      if (rest.length) next.set(conversationId, rest)
      else next.delete(conversationId)
      return next
    })
  }, [])

  const clearQueuedMessages = useCallback((conversationId: string) => {
    setQueuedMessages((prev) => {
      if (!prev.has(conversationId)) return prev
      const next = new Map(prev)
      next.delete(conversationId)
      return next
    })
  }, [])

  const markConversationBusy = useCallback((conversationId: string) => {
    if (!conversationId) return
    setBusyConversations((prev) => {
      if (prev.has(conversationId)) return prev
      const next = new Set(prev)
      next.add(conversationId)
      return next
    })
  }, [])

  const clearConversationBusy = useCallback((conversationId: string) => {
    if (!conversationId) return
    setBusyConversations((prev) => {
      if (!prev.has(conversationId)) return prev
      const next = new Set(prev)
      next.delete(conversationId)
      return next
    })
  }, [])

  const isConversationBusy = useCallback(
    (conversationId: string) => busyConversations.has(conversationId),
    [busyConversations]
  )

  useEffect(() => {
    const ws = wsRef.current!
    if (token) {
      ws.close()
      ws.connect(token, currentTeamId)
    } else {
      ws.close()
    }
    setBusyConversations(new Set())
    setQueuedMessages(new Map())
    return () => ws.close()
  }, [token, currentTeamId])

  useEffect(() => {
    const ws = wsRef.current!
    // A turn that finishes with messages still queued leaves the conversation
    // busy: the next turn starts on its own, and clearing busy in between would
    // flicker the composer back to idle for the length of one round trip.
    const onCompleted = (payload: CompletedPayload) => {
      const id = payload?.conversation_id
      if (!id) return
      if (!payload.queued_remaining) clearConversationBusy(id)
    }
    const onQueued = (payload: QueuedPayload) => {
      const id = payload?.conversation_id
      if (!id || typeof payload.content !== "string") return
      markConversationBusy(id)
      enqueueMessage(id, payload.content)
    }
    const onDequeued = (payload: QueuedPayload) => {
      const id = payload?.conversation_id
      if (!id || typeof payload.content !== "string") return
      markConversationBusy(id)
      dequeueMessage(id, payload.content)
    }
    // A refused message is not a failed turn: the one in flight is still running,
    // and so is everything already queued behind it.
    const onError = (payload: ErrorPayload) => {
      const id = payload?.conversation_id
      if (!id || payload.code === "queue_full") return
      clearConversationBusy(id)
      clearQueuedMessages(id)
    }
    ws.on("conversation.message.completed", onCompleted)
    ws.on("conversation.message.queued", onQueued)
    ws.on("conversation.message.dequeued", onDequeued)
    ws.on("conversation.error", onError)
    return () => {
      ws.off("conversation.message.completed", onCompleted)
      ws.off("conversation.message.queued", onQueued)
      ws.off("conversation.message.dequeued", onDequeued)
      ws.off("conversation.error", onError)
    }
  }, [
    clearConversationBusy,
    markConversationBusy,
    enqueueMessage,
    dequeueMessage,
    clearQueuedMessages,
  ])

  return (
    <WebSocketContext.Provider
      value={{
        ws: wsRef.current,
        markConversationBusy,
        isConversationBusy,
        busyConversations,
        queuedMessages,
      }}
    >
      {children}
    </WebSocketContext.Provider>
  )
}

export function useWebSocket(): BuildMaxWebSocket {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error("useWebSocket must be used within WebSocketProvider")
  return ctx.ws
}

export function useConversationBusy(conversationId: string | null | undefined): {
  busy: boolean
  markBusy: () => void
  busyConversations: ReadonlySet<string>
  queued: string[]
} {
  const ctx = useContext(WebSocketContext)
  if (!ctx)
    throw new Error("useConversationBusy must be used within WebSocketProvider")
  // markConversationBusy is stable (useCallback []). Bind conversationId via useCallback
  // so the returned markBusy is stable across renders for a given conversationId — otherwise
  // consumers that include markBusy in effect/callback deps would churn every render.
  const { markConversationBusy } = ctx
  const markBusy = useCallback(() => {
    if (conversationId) markConversationBusy(conversationId)
  }, [conversationId, markConversationBusy])
  return {
    busy: conversationId ? ctx.isConversationBusy(conversationId) : false,
    markBusy,
    busyConversations: ctx.busyConversations,
    queued: (conversationId && ctx.queuedMessages.get(conversationId)) || [],
  }
}
