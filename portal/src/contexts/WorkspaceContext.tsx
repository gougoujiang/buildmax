import {
  createContext,
  useContext,
  type ReactNode,
} from "react"
import type { Artifact, Route, Chat } from "../lib/types"
import type { ApiWorkspace } from "../lib/api"
import { getWorkspaceScope, useHashRoute } from "../router"
import { useAuth } from "./AuthContext"
import { useWorkspaceData } from "../hooks/useWorkspaceData"

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
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const route = useHashRoute()
  const data = useWorkspaceData(token, route)
  const scope = getWorkspaceScope(route)

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
