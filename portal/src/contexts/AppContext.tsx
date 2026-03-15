import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { Route, Task } from "../lib/types"
import { getScope, useHashRoute } from "../router"
import { useAuth } from "./AuthContext"

export interface PendingTask {
  task: Task
  initialInput: string
}

export interface AppContextValue {
  token: string | null
  route: Route
  scope: ReturnType<typeof getScope>
  /** Set when navigating so TaskDetail can render immediately and show the initial query. */
  pendingTask: PendingTask | null
  setPendingTask: (p: PendingTask | null) => void
}

const AppContext = createContext<AppContextValue | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const scope = getScope(route)
  const [pendingTask, setPendingTaskState] = useState<PendingTask | null>(null)
  const setPendingTask = useCallback((p: PendingTask | null) => {
    setPendingTaskState(p)
  }, [])

  const value: AppContextValue = {
    token,
    route,
    scope,
    pendingTask,
    setPendingTask,
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
