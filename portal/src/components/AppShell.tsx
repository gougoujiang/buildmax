import type { ReactNode } from "react"
import type { Workspace } from "../lib/types"
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
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function AppShell({
  currentWorkspace,
  workspaces,
  onNewWorkspace,
  user,
  onLogout,
  children,
}: AppShellProps) {
  const { route, projects } = useWorkspace()

  return (
    <div className="shell">
      <TopBar
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        onWorkspaceChange={(workspaceId) => navigate({ name: "workspace", workspaceId })}
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
          <Breadcrumbs route={route} projects={projects} />
          {children}
        </main>
      </div>
    </div>
  )
}