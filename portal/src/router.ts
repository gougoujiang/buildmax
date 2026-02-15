import { useState, useEffect } from "react"
import type { Route } from "./types"

/**
 * Parse window.location.hash into a typed Route.
 * Hash format: #<workspaceId> | #<workspaceId>/project/<id> | #<workspaceId>/task/<p>/<t> | #<workspaceId>/artifact/<p>/<id>
 * First segment = workspaceId (use as-is; if missing, "").
 */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  const workspaceId = parts[0] ?? ""

  if (parts[1] === "project" && parts[2]) {
    return { name: "project", workspaceId, projectId: parts[2] }
  }
  if (parts[1] === "task" && parts[2] && parts[3]) {
    return {
      name: "task",
      workspaceId,
      projectId: parts[2],
      taskId: parts[3],
    }
  }
  if (parts[1] === "artifact" && parts[2] && parts[3]) {
    return {
      name: "artifact",
      workspaceId,
      projectId: parts[2],
      artifactId: parts[3],
    }
  }
  return { name: "workspace", workspaceId }
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "workspace":
      return `#${route.workspaceId}`
    case "project":
      return `#${route.workspaceId}/project/${route.projectId}`
    case "task":
      return `#${route.workspaceId}/task/${route.projectId}/${route.taskId}`
    case "artifact":
      return `#${route.workspaceId}/artifact/${route.projectId}/${route.artifactId}`
  }
}

/** Navigate to a Route by setting the hash. */
export function navigate(route: Route): void {
  window.location.hash = buildHash(route)
}

/** React hook: returns the current Route and re-renders on hashchange. */
export function useHashRoute(): Route {
  const [route, setRoute] = useState<Route>(() =>
    parseHash(window.location.hash),
  )

  useEffect(() => {
    function onHashChange() {
      setRoute(parseHash(window.location.hash))
    }
    window.addEventListener("hashchange", onHashChange)
    return () => window.removeEventListener("hashchange", onHashChange)
  }, [])

  return route
}
