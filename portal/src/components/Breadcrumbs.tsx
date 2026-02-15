import type { Route, Project } from "../lib/types"
import { navigate } from "../lib/router"
import { getProjectById } from "../data/mockData"

interface BreadcrumbsProps {
  route: Route
}

export function Breadcrumbs({ route }: BreadcrumbsProps) {
  const workspaceId = route.workspaceId
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "activity") {
    crumbs = [{ label: "Activity", route: { name: "activity", workspaceId } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Explore", route: { name: "explore", workspaceId } }]
  } else {
    // workspace, project, task, artifact — all under "Projects"
    crumbs = [{ label: "Projects", route: { name: "workspace", workspaceId } }]
    if (route.name !== "workspace") {
      const projectId = route.projectId
      const project: Project | undefined = getProjectById(projectId)
      crumbs.push({
        label: project?.name ?? "Project",
        route: { name: "project", workspaceId, projectId },
      })
      if (route.name === "task") {
        crumbs.push({ label: "Task", route })
      }
      if (route.name === "artifact") {
        crumbs.push({ label: "Artifact", route })
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
