import type { ReactNode } from "react"
import type { Task, Workspace } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../lib/router"
import { useWorkspace } from "../contexts/WorkspaceContext"
import { TopBar } from "./TopBar"
import { LeftSidebar } from "./LeftSidebar"
import { Breadcrumbs } from "./Breadcrumbs"

interface AppShellProps {
  currentWorkspace: Workspace
  workspaces: { id: string; name: string }[]
  onNewWorkspace?: () => void
  workspaceTasks: Task[]
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function AppShell({
  currentWorkspace,
  workspaces,
  onNewWorkspace,
  workspaceTasks,
  user,
  onLogout,
  children,
}: AppShellProps) {
  const { route } = useWorkspace()

  return (
    <div className="shell">
      <TopBar user={user} onLogout={onLogout} />
      <div className="shell__body">
        <LeftSidebar
          workspaceId={route.workspaceId}
          route={route}
          currentWorkspace={currentWorkspace}
          workspaces={workspaces}
          onWorkspaceChange={(workspaceId) => navigate({ name: "workspace", workspaceId })}
          onNewWorkspace={onNewWorkspace}
          workspaceTasks={workspaceTasks}
        />
        <main className="shell__main">
          <Breadcrumbs route={route} />
          {children}
        </main>
      </div>
    </div>
  )
}