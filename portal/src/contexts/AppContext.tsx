import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { BreadcrumbCrumb, Route } from "../lib/types"
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
  /**
   * Origin-aware breadcrumb trails a detail page has computed for a leaf whose
   * parents cannot be derived from the route alone — a Task links back to its
   * agent, issue, or conversation. Keyed by the leaf id.
   */
  breadcrumbTrails: Record<string, BreadcrumbCrumb[]>
  setBreadcrumbTrail: (id: string, trail: BreadcrumbCrumb[]) => void
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

  const [breadcrumbTrails, setBreadcrumbTrails] = useState<Record<string, BreadcrumbCrumb[]>>({})
  const setBreadcrumbTrail = useCallback((id: string, trail: BreadcrumbCrumb[]) => {
    setBreadcrumbTrails((prev) => {
      const cur = prev[id]
      // Skip the update when the labels are unchanged, so a re-publish (e.g. an
      // agent name arriving) does not churn every breadcrumb consumer.
      if (cur && cur.length === trail.length && cur.every((c, i) => c.label === trail[i].label)) {
        return prev
      }
      return { ...prev, [id]: trail }
    })
  }, [])

  const value: AppContextValue = {
    token,
    route,
    pendingConversation,
    setPendingConversation,
    entityLabels,
    setEntityLabel,
    breadcrumbTrails,
    setBreadcrumbTrail,
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
