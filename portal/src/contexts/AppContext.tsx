import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { Route } from "../lib/types"
import { useHashRoute } from "../router"
import { useAuth } from "./AuthContext"

export interface PendingConversation {
  conversationId: string
  initialMessage: string
}

export interface AppContextValue {
  token: string | null
  route: Route
  /** Set when creating a new conversation so ConversationDetail can send the first message on mount. */
  pendingConversation: PendingConversation | null
  setPendingConversation: (p: PendingConversation | null) => void
}

const AppContext = createContext<AppContextValue | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const [pendingConversation, setPendingConversationState] = useState<PendingConversation | null>(null)
  const setPendingConversation = useCallback((p: PendingConversation | null) => {
    setPendingConversationState(p)
  }, [])

  const value: AppContextValue = {
    token,
    route,
    pendingConversation,
    setPendingConversation,
  }

  return (
    <AppContext.Provider value={value}>
      {children}
    </AppContext.Provider>
  )
}

export function useApp(): AppContextValue {
  const ctx = useContext(AppContext)
  if (!ctx) throw new Error("useApp must be used within AppProvider")
  return ctx
}
