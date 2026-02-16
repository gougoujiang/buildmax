import type { Route } from "../lib/types"
import { navigate } from "../lib/router"

interface LeftSidebarProps {
  workspaceId: string
  route: Route
}

type SidebarNavName = "workspace" | "activity" | "explore"

const SIDEBAR_NAV: { label: string; name: SidebarNavName }[] = [
  { label: "Explore", name: "explore" },
  { label: "Projects", name: "workspace" },
  { label: "Activity", name: "activity" },
]

function isNavActive(route: Route, targetName: SidebarNavName): boolean {
  if (targetName === "workspace") {
    return (
      route.name === "workspace" ||
      route.name === "project" ||
      route.name === "task"
    )
  }
  return route.name === targetName
}

function navToRoute(name: SidebarNavName, workspaceId: string): Route {
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
