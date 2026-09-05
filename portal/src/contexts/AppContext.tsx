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
  /**
   * Human labels for entity ids a detail page has loaded, so the breadcrumb can
   * show a name instead of an opaque id. A detail page publishes its entity here
   * once loaded; a reader falls back to the id until then.
   */
  entityLabels: Record<string, string>
  setEntityLabel: (id: string, label: string) => void
}

const AppContext = createContext<AppContextValue | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const [pendingConversation, setPendingConversationState] = useState<PendingConversation | null>(null)
  const setPendingConversation = useCallback((p: PendingConversation | null) => {
    setPendingConversationState(p)
  }, [])

  const [entityLabels, setEntityLabels] = useState<Record<string, string>>({})
  const setEntityLabel = useCallback((id: string, label: string) => {
    setEntityLabels((prev) => (prev[id] === label ? prev : { ...prev, [id]: label }))
  }, [])

  const value: AppContextValue = {
    token,
    route,
    pendingConversation,
    setPendingConversation,
    entityLabels,
    setEntityLabel,
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
