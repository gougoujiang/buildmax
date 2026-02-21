import type { Route } from "../lib/types"
import { navigate } from "../lib/router"

interface BreadcrumbsProps {
  route: Route
}

export function Breadcrumbs({ route }: BreadcrumbsProps) {
  const workspaceId = route.workspaceId
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "newChat") {
    crumbs = [{ label: "New Chat", route: { name: "newChat", workspaceId } }]
  } else if (route.name === "activity") {
    crumbs = [{ label: "Activity", route: { name: "activity", workspaceId } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Explore", route: { name: "explore", workspaceId } }]
  } else if (route.name === "agents") {
    crumbs = [{ label: "Agents", route: { name: "agents", workspaceId } }]
  } else if (route.name === "agentList") {
    crumbs = [
      { label: "Agents", route: { name: "agents", workspaceId } },
      { label: "Manage agents", route: { name: "agentList", workspaceId } },
    ]
  } else if (route.name === "task") {
    crumbs = [
      { label: "Chats", route: { name: "activity", workspaceId } },
      { label: "Task", route },
    ]
  } else {
    crumbs = [{ label: "Home", route: { name: "workspace", workspaceId } }]
  }

  return (
    <nav className="breadcrumbs" aria-label="Breadcrumb">
      {crumbs.map((crumb, i) => {
        const isLast = i === crumbs.length - 1
        return (
          <span key={i} className="breadcrumbs__segment">
            {isLast ? (
              <span className="breadcrumbs__current">{crumb.label}</span>
            ) : (
              <>
                <button
                  type="button"
                  className="breadcrumbs__link"
                  onClick={() => navigate(crumb.route)}
                >
                  {crumb.label}
                </button>
                <span className="breadcrumbs__separator">/</span>
              </>
            )}
          </span>
        )
      })}
    </nav>
  )
}
