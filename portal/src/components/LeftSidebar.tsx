import type { Route } from "../lib/types"
import { navigate } from "../lib/router"

interface LeftSidebarProps {
  workspaceId: string
  route: Route
}

type SidebarNavName = "workspace" | "activity" | "explore" | "agents"

const SIDEBAR_NAV: { label: string; name: SidebarNavName }[] = [
  { label: "Projects", name: "workspace" },
  { label: "Explore", name: "explore" },
  { label: "Activity", name: "activity" },
  { label: "Agents", name: "agents" },
]

function isNavActive(route: Route, targetName: SidebarNavName): boolean {
  if (targetName === "workspace") {
    return (
      route.name === "workspace" ||
      route.name === "project" ||
      route.name === "task"
    )
  }
  if (targetName === "agents") {
    return route.name === "agents" || route.name === "agentList"
  }
  return route.name === targetName
}

function navToRoute(name: SidebarNavName, workspaceId: string): Route {
  if (name === "agents") return { name: "agents", workspaceId }
  return { name, workspaceId }
}

export function LeftSidebar({
  workspaceId,
  route,
}: LeftSidebarProps) {
  return (
    <aside className="sidebar">
      <nav className="sidebar__nav" aria-label="Primary">
        {SIDEBAR_NAV.map(({ label, name }) => {
          const isActive = isNavActive(route, name)
          const targetRoute = navToRoute(name, workspaceId)
          return (
            <button
              key={name}
              type="button"
              className={
                "sidebar__nav-item" +
                (isActive ? " sidebar__nav-item--active" : "")
              }
              onClick={() => navigate(targetRoute)}
            >
              {label}
            </button>
          )
        })}
      </nav>
    </aside>
  )
}
