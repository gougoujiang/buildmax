import type { ReactNode } from "react"
import type { Workspace, Project, Route } from "../types"
import { TopBar } from "./TopBar"
import { ProjectsSidebar } from "./ProjectsSidebar"
import { Breadcrumbs } from "./Breadcrumbs"

interface AppShellProps {
  currentWorkspace: Workspace
  workspaces: Workspace[]
  projects: Project[]
  selectedProjectId: string | null
  route: Route
  onWorkspaceChange: (workspaceId: string) => void
  children: ReactNode
}

export function AppShell({
  currentWorkspace,
  workspaces,
  projects,
  selectedProjectId,
  route,
  onWorkspaceChange,
  children,
}: AppShellProps) {
  return (
    <div className="shell">
      <TopBar
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        onWorkspaceChange={onWorkspaceChange}
      />
      <div className="shell__body">
        <ProjectsSidebar
          workspaceId={route.workspaceId}
          projects={projects}
          selectedProjectId={selectedProjectId}
        />
        <main className="shell__main">
          <Breadcrumbs route={route} />
          {children}
        </main>
      </div>
    </div>
  )
}
