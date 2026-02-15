import { useState, useEffect } from "react"
import type { Route } from "./types"

/**
 * Parse window.location.hash into a typed Route.
 * Supported patterns:
 *   (empty) | #workspace         → { name: "workspace" }
 *   #project/<id>                → { name: "project", projectId }
 *   #task/<projectId>/<taskId>   → { name: "task", projectId, taskId }
 *   #artifact/<projectId>/<id>   → { name: "artifact", projectId, artifactId }
 */
export function parseHash(hash: string): Route {
  // Strip leading "#" or "#/"
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  if (parts[0] === "project" && parts[1]) {
    return { name: "project", projectId: parts[1] }
  }
  if (parts[0] === "task" && parts[1] && parts[2]) {
    return { name: "task", projectId: parts[1], taskId: parts[2] }
  }
  if (parts[0] === "artifact" && parts[1] && parts[2]) {
    return { name: "artifact", projectId: parts[1], artifactId: parts[2] }
  }
  return { name: "workspace" }
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "workspace":
      return "#workspace"
    case "project":
      return `#project/${route.projectId}`
    case "task":
      return `#task/${route.projectId}/${route.taskId}`
    case "artifact":
      return `#artifact/${route.projectId}/${route.artifactId}`
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
