import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react"
import type { Artifact, Route, Chat } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { getWorkspaceScope, useHashRoute } from "../router"
import { useAuth } from "./AuthContext"
import { useWorkspaceData } from "../hooks/useWorkspaceData"

export interface PendingChat {
  chat: Chat
  initialInput: string
}

export interface WorkspaceContextValue {
  token: string | null
  route: Route
  scope: ReturnType<typeof getWorkspaceScope>
  workspaces: ApiWorkspace[]
  workspaceChats: Chat[]
  artifacts: Artifact[]
  loadingWorkspaces: boolean
  refetchWorkspaces: () => Promise<void>
  refetchWorkspaceChats: () => Promise<void>
  refetchArtifacts: (chatId?: string) => void
  /** Set when navigating from New Chat so ChatDetail can render immediately and show initial query. */
  pendingChat: PendingChat | null
  setPendingChat: (p: PendingChat | null) => void
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const data = useWorkspaceData(token, route)
  const scope = getWorkspaceScope(route)
  const [pendingChat, setPendingChatState] = useState<PendingChat | null>(null)
  const setPendingChat = useCallback((p: PendingChat | null) => {
    setPendingChatState(p)
  }, [])

  const value: WorkspaceContextValue = {
    token,
    route,
    scope,
    workspaces: data.workspaces,
    workspaceChats: data.workspaceChats,
    artifacts: data.artifacts,
    loadingWorkspaces: data.loadingWorkspaces,
    refetchWorkspaces: data.refetchWorkspaces,
    refetchWorkspaceChats: data.refetchWorkspaceChats,
    refetchArtifacts: data.refetchArtifacts,
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
