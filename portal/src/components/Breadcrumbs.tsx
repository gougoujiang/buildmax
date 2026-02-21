import type { Route, Project } from "../lib/types"
import { navigate } from "../lib/router"

interface BreadcrumbsProps {
  route: Route
  projects: Project[]
}

export function Breadcrumbs({ route, projects }: BreadcrumbsProps) {
  const workspaceId = route.workspaceId
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "activity") {
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
  } else {
    // workspace, project, task — all under "Projects"
    crumbs = [{ label: "Projects", route: { name: "workspace", workspaceId } }]
    if (route.name === "project" || route.name === "task") {
      const projectId = route.projectId
      const project = projects.find((p) => p.id === projectId)
      crumbs.push({
        label: project?.name ?? "Project",
        route: { name: "project", workspaceId, projectId },
      })
      if (route.name === "task") {
        crumbs.push({ label: "Task", route })
      }
    }
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
