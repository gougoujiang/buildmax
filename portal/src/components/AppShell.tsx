import type { ReactNode } from "react"
import type { Project, Route } from "../types"
import { TopBar } from "./TopBar"
import { ProjectsSidebar } from "./ProjectsSidebar"
import { Breadcrumbs } from "./Breadcrumbs"

interface AppShellProps {
  workspaceName: string
  projects: Project[]
  selectedProjectId: string | null
  route: Route
  children: ReactNode
}

export function AppShell({
  workspaceName,
  projects,
  selectedProjectId,
  route,
  children,
}: AppShellProps) {
  return (
    <div className="shell">
      <TopBar workspaceName={workspaceName} />
      <div className="shell__body">
        <ProjectsSidebar
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
