import {
  createContext,
  useContext,
  useEffect,
  useRef,
  type ReactNode,
} from "react"
import { BuildMaxWebSocket } from "../lib/api/ws"
import { useAuth } from "./AuthContext"
import { useTeam } from "./TeamContext"

const WebSocketContext = createContext<BuildMaxWebSocket | null>(null)

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const { currentTeamId } = useTeam()
  const wsRef = useRef<BuildMaxWebSocket | null>(null)

  if (!wsRef.current) {
    wsRef.current = new BuildMaxWebSocket()
  }

  useEffect(() => {
    const ws = wsRef.current!
    if (token) {
      ws.close()
      ws.connect(token, currentTeamId)
    } else {
      ws.close()
    }
    return () => ws.close()
  }, [token, currentTeamId])

  return (
    <WebSocketContext.Provider value={wsRef.current}>
      {children}
    </WebSocketContext.Provider>
  )
}

export function useWebSocket(): BuildMaxWebSocket {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error("useWebSocket must be used within WebSocketProvider")
  return ctx
}
