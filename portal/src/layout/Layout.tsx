import type { ReactNode } from "react"
import type { Conversation, Route, Workspace } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { Sidebar } from "./Sidebar"
import { Breadcrumbs } from "./Breadcrumbs"
import { ThemeToggle } from "@buildmax/gui"

export interface LayoutProps {
  route: Route
  currentWorkspace: Workspace
  workspaces: { id: string; name: string }[]
  onNewWorkspace?: () => void
  workspaceConversations: Conversation[]
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function Layout({
  route,
  currentWorkspace,
  workspaces,
  onNewWorkspace,
  workspaceConversations,
  user,
  onLogout,
  children,
}: LayoutProps) {
  return (
    <div className="shell">
      <div className="shell__body">
        <Sidebar
          workspaceId={route.workspaceId}
          route={route}
          currentWorkspace={currentWorkspace}
          workspaces={workspaces}
          onWorkspaceChange={(workspaceId) => navigate({ name: "workspace", workspaceId })}
          onNewWorkspace={onNewWorkspace}
          workspaceConversations={workspaceConversations}
          user={user}
          onLogout={onLogout}
        />
        <main className="shell__main">
          <div className="shell__top">
            <Breadcrumbs route={route} workspaceConversations={workspaceConversations} />
            <ThemeToggle />
          </div>
          <div className="shell__content">{children}</div>
        </main>
      </div>
    </div>
  )
}
