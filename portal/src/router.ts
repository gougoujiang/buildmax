import { useState, useEffect } from "react"
import type { Route, WorkspaceScope } from "./lib/types"

/** Derive workspace scope from route (for data fetching and display). */
export function getWorkspaceScope(route: Route): WorkspaceScope {
  return {
    workspaceId: route.workspaceId,
    taskId: "taskId" in route ? route.taskId : undefined,
  }
}

/** Path segment names used in the hash URL. Single source of truth for parseHash/buildHash. */
export const SEGMENT = {
  new: "new",
  task: "task",
  activity: "activity",
  explore: "explore",
  agents: "agents",
} as const

/**
 * Parse window.location.hash into a typed Route.
 * Hash format: #<workspaceId> | #<workspaceId>/task/<taskId> | ...
 * First segment = workspaceId (use as-is; if missing, "").
 */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  const workspaceId = parts[0] ?? ""

  if (parts[1] === SEGMENT.new) {
    return { name: "newChat", workspaceId }
  }
  if (parts[1] === SEGMENT.activity) {
    return { name: "activity", workspaceId }
  }
  if (parts[1] === SEGMENT.explore) {
    return { name: "explore", workspaceId }
  }
  if (parts[1] === SEGMENT.agents && parts[2] === "list") {
    return { name: "agentList", workspaceId }
  }
  if (parts[1] === SEGMENT.agents) {
    return { name: "agents", workspaceId }
  }
  if (parts[1] === SEGMENT.task && parts[2]) {
    return { name: "task", workspaceId, taskId: parts[2] }
  }
  return { name: "workspace", workspaceId }
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "workspace":
      return `#${route.workspaceId}`
    case "newChat":
      return `#${route.workspaceId}/${SEGMENT.new}`
    case "task":
      return `#${route.workspaceId}/${SEGMENT.task}/${route.taskId}`
    case "activity":
      return `#${route.workspaceId}/${SEGMENT.activity}`
    case "explore":
      return `#${route.workspaceId}/${SEGMENT.explore}`
    case "agents":
      return `#${route.workspaceId}/${SEGMENT.agents}`
    case "agentList":
      return `#${route.workspaceId}/${SEGMENT.agents}/list`
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
