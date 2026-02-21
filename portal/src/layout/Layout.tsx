import type { ReactNode } from "react"
import type { Chat, Workspace } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { LeftSidebar } from "./LeftSidebar"
import { Breadcrumbs } from "./Breadcrumbs"

export interface LayoutProps {
  currentWorkspace: Workspace
  workspaces: { id: string; name: string }[]
  onNewWorkspace?: () => void
  workspaceChats: Chat[]
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function Layout({
  currentWorkspace,
  workspaces,
  onNewWorkspace,
  workspaceChats,
  user,
  onLogout,
  children,
}: LayoutProps) {
  const { route } = useWorkspace()

  return (
    <div className="shell">
      <div className="shell__body">
        <LeftSidebar
          workspaceId={route.workspaceId}
          route={route}
          currentWorkspace={currentWorkspace}
          workspaces={workspaces}
          onWorkspaceChange={(workspaceId) => navigate({ name: "workspace", workspaceId })}
          onNewWorkspace={onNewWorkspace}
          workspaceChats={workspaceChats}
          user={user}
          onLogout={onLogout}
        />
        <main className="shell__main">
          <Breadcrumbs route={route} />
          {children}
        </main>
      </div>
    </div>
  )
}
