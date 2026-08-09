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
    return () => ws.close()
  }, [token, currentTeamId])

  useEffect(() => {
    const ws = wsRef.current!
    const onCompleted = (payload: { conversation_id?: string }) => {
      if (payload?.conversation_id) clearConversationBusy(payload.conversation_id)
    }
    const onError = (payload: { conversation_id?: string }) => {
      if (payload?.conversation_id) clearConversationBusy(payload.conversation_id)
    }
    ws.on("conversation.message.completed", onCompleted)
    ws.on("conversation.error", onError)
    return () => {
      ws.off("conversation.message.completed", onCompleted)
      ws.off("conversation.error", onError)
    }
  }, [clearConversationBusy])

  return (
    <WebSocketContext.Provider
      value={{
        ws: wsRef.current,
        markConversationBusy,
        isConversationBusy,
        busyConversations,
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
  }
}
