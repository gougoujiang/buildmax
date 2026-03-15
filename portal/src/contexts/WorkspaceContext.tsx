import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { Route, Chat } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { getWorkspaceScope, useHashRoute } from "../router"
import { useAuth } from "./AuthContext"
import { useWorkspaces } from "../hooks/useWorkspaces"

export interface PendingChat {
  chat: Chat
  initialInput: string
}

export interface WorkspaceContextValue {
  token: string | null
  route: Route
  scope: ReturnType<typeof getWorkspaceScope>
  workspaces: ApiWorkspace[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => Promise<void>
  /** Set when navigating from New Conversation so TaskDetail can render immediately and show the initial query. */
  pendingChat: PendingChat | null
  setPendingChat: (p: PendingChat | null) => void
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const scope = getWorkspaceScope(route)
  const {
    data: workspaces,
    loading: loadingWorkspaces,
    refetch: refetchWorkspaces,
  } = useWorkspaces(token)
  const [pendingChat, setPendingChatState] = useState<PendingChat | null>(null)
  const setPendingChat = useCallback((p: PendingChat | null) => {
    setPendingChatState(p)
  }, [])

  const value: WorkspaceContextValue = {
    token,
    route,
    scope,
    workspaces,
    loadingWorkspaces,
    refetchWorkspaces,
    pendingChat,
    setPendingChat,
  }

  return (
    <WorkspaceContext.Provider value={value}>
      {children}
    </WorkspaceContext.Provider>
  )
}

export function useWorkspace(): WorkspaceContextValue {
  const ctx = useContext(WorkspaceContext)
  if (!ctx) throw new Error("useWorkspace must be used within WorkspaceProvider")
  return ctx
}
