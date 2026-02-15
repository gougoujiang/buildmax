import type { Route } from "../types"
import { navigate } from "../router"

interface LeftSidebarProps {
  workspaceId: string
  route: Route
}

export function LeftSidebar({
  workspaceId,
  route,
}: LeftSidebarProps) {
  const projectsActive =
    route.name === "workspace" ||
    route.name === "project" ||
    route.name === "task" ||
    route.name === "artifact"
  const activityActive = route.name === "activity"
  const exploreActive = route.name === "explore"

  return (
    <aside className="sidebar">
      <nav className="sidebar__nav" aria-label="Primary">
        <button
          type="button"
          className={
            "sidebar__nav-item" + (projectsActive ? " sidebar__nav-item--active" : "")
          }
          onClick={() => navigate({ name: "workspace", workspaceId })}
        >
          Projects
        </button>
        <button
          type="button"
          className={
            "sidebar__nav-item" +
            (activityActive ? " sidebar__nav-item--active" : "")
          }
          onClick={() => navigate({ name: "activity", workspaceId })}
        >
          Activity
        </button>
        <button
          type="button"
          className={
            "sidebar__nav-item" +
            (exploreActive ? " sidebar__nav-item--active" : "")
          }
          onClick={() => navigate({ name: "explore", workspaceId })}
        >
          Explore
        </button>
      </nav>
    </aside>
  )
}
