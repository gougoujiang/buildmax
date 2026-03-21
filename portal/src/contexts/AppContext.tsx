import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { Route } from "../lib/types"
import { getScope, useHashRoute } from "../router"
import { useAuth } from "./AuthContext"

export interface PendingConversation {
  conversationId: string
  initialMessage: string
}

export interface AppContextValue {
  token: string | null
  route: Route
  scope: ReturnType<typeof getScope>
  /** Set when creating a new conversation so ConversationDetail can send the first message on mount. */
  pendingConversation: PendingConversation | null
  setPendingConversation: (p: PendingConversation | null) => void
}

const AppContext = createContext<AppContextValue | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const scope = getScope(route)
  const [pendingConversation, setPendingConversationState] = useState<PendingConversation | null>(null)
  const setPendingConversation = useCallback((p: PendingConversation | null) => {
    setPendingConversationState(p)
  }, [])

  const value: AppContextValue = {
    token,
    route,
    scope,
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
