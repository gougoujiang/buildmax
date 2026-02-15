import type { ReactNode } from "react"
import type { Workspace, Route } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { TopBar } from "./TopBar"
import { LeftSidebar } from "./LeftSidebar"
import { Breadcrumbs } from "./Breadcrumbs"

interface AppShellProps {
  currentWorkspace: Workspace
  workspaces: Workspace[]
  route: Route
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function AppShell({
  currentWorkspace,
  workspaces,
  route,
  onWorkspaceChange,
  onNewWorkspace,
  user,
  onLogout,
  children,
}: AppShellProps) {
  return (
    <div className="shell">
      <TopBar
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        onWorkspaceChange={onWorkspaceChange}
        onNewWorkspace={onNewWorkspace}
        user={user}
        onLogout={onLogout}
      />
      <div className="shell__body">
        <LeftSidebar
          workspaceId={route.workspaceId}
          route={route}
        />
        <main className="shell__main">
          <Breadcrumbs route={route} />
          {children}
        </main>
      </div>
    </div>
  )
}
