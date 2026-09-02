import { useState, useEffect } from "react"
import type { Route } from "./lib/types"

/**
 * Path segment names used in the hash URL. Single source of truth for parseHash/buildHash.
 */
export const SEGMENT = {
  login: "login",
  conversation: "conversation",
  conversations: "conversations",
  explore: "explore",
  agents: "agents",
  account: "account",
  space: "space",
  admin: "admin",
  teamSettings: "team-settings",
  workflows: "workflows",
  workflow: "workflow",
  workflowRun: "workflow-run",
  issues: "issues",
  issue: "issue",
} as const

/**
 * Parse window.location.hash into a typed Route.
 * Hash format: #/ | #/conversation/<conversationId> | #/conversations | ...
 */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  if (parts[0] === SEGMENT.login) {
    return { name: "login" }
  }
  if (parts[0] === SEGMENT.conversations) {
    return { name: "conversations" }
  }
  if (parts[0] === SEGMENT.explore) {
    return { name: "explore" }
  }
  if (parts[0] === SEGMENT.agents) {
    return { name: "agents" }
  }
  if (parts[0] === SEGMENT.account) {
    if (parts[1] === "usage") return { name: "account", section: "usage" }
    if (parts[1] === "webhook") return { name: "account", section: "webhook" }
    if (parts[1] === "plugins") return { name: "account", section: "plugins" }
    if (parts[1] === "invitations") return { name: "account", section: "invitations" }
    return { name: "account", section: "general" }
  }
  if (parts[0] === SEGMENT.space) {
    if (parts[1] === "members" && parts[2] === "new") return { name: "space", section: "memberNew" }
    if (parts[1] === "members") return { name: "space", section: "members" }
    if (parts[1] === "artifacts") return { name: "space", section: "artifacts" }
    if (parts[1] === "plugins") return { name: "space", section: "plugins" }
    if (parts[1] === "secrets") return { name: "space", section: "secrets" }
    if (parts[1] === "audit") return { name: "space", section: "audit" }
    return { name: "space", section: "overview" }
  }
  if (parts[0] === SEGMENT.admin) {
    if (parts[1] === "accounts") return { name: "admin", section: "accounts" }
    if (parts[1] === "teams") return { name: "admin", section: "teams" }
    if (parts[1] === "models") return { name: "admin", section: "models" }
    if (parts[1] === "plugins") return { name: "admin", section: "plugins" }
    if (parts[1] === "audit") return { name: "admin", section: "audit" }
    return { name: "admin", section: "overview" }
  }
  if (parts[0] === SEGMENT.teamSettings) {
    return { name: "space", section: "overview" }
  }
  if (parts[0] === SEGMENT.workflows) {
    return { name: "workflows" }
  }
  if (parts[0] === SEGMENT.workflow && parts[1]) {
    return { name: "workflow", workflowId: parts[1] }
  }
  if (parts[0] === SEGMENT.workflowRun && parts[1]) {
    return { name: "workflowRun", workflowRunId: parts[1] }
  }
  if (parts[0] === SEGMENT.issues) {
    return { name: "issues" }
  }
  if (parts[0] === SEGMENT.issue && parts[1]) {
    return { name: "issue", issueId: parts[1] }
  }
  if (parts[0] === SEGMENT.conversation && parts[1]) {
    return { name: "conversation", conversationId: parts[1] }
  }
  return { name: "home" }
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "home":
      return "#/"
    case "login":
      return `#/${SEGMENT.login}`
    case "conversation":
      return `#/${SEGMENT.conversation}/${route.conversationId}`
    case "conversations":
      return `#/${SEGMENT.conversations}`
    case "explore":
      return `#/${SEGMENT.explore}`
    case "agents":
      return `#/${SEGMENT.agents}`
    case "account":
      switch (route.section) {
        case "usage":
          return `#/${SEGMENT.account}/usage`
        case "webhook":
          return `#/${SEGMENT.account}/webhook`
        case "plugins":
          return `#/${SEGMENT.account}/plugins`
        case "invitations":
          return `#/${SEGMENT.account}/invitations`
        case "general":
        default:
          return `#/${SEGMENT.account}`
      }
    case "space":
      switch (route.section) {
        case "members":
          return `#/${SEGMENT.space}/members`
        case "memberNew":
          return `#/${SEGMENT.space}/members/new`
        case "artifacts":
          return `#/${SEGMENT.space}/artifacts`
        case "plugins":
          return `#/${SEGMENT.space}/plugins`
        case "secrets":
          return `#/${SEGMENT.space}/secrets`
        case "audit":
          return `#/${SEGMENT.space}/audit`
        case "overview":
        default:
          return `#/${SEGMENT.space}`
      }
    case "admin":
      switch (route.section) {
        case "accounts":
          return `#/${SEGMENT.admin}/accounts`
        case "teams":
          return `#/${SEGMENT.admin}/teams`
        case "models":
          return `#/${SEGMENT.admin}/models`
        case "plugins":
          return `#/${SEGMENT.admin}/plugins`
        case "audit":
          return `#/${SEGMENT.admin}/audit`
        case "overview":
        default:
          return `#/${SEGMENT.admin}`
      }
    case "workflows":
      return `#/${SEGMENT.workflows}`
    case "workflow":
      return `#/${SEGMENT.workflow}/${route.workflowId}`
    case "workflowRun":
      return `#/${SEGMENT.workflowRun}/${route.workflowRunId}`
    case "issues":
      return `#/${SEGMENT.issues}`
    case "issue":
      return `#/${SEGMENT.issue}/${route.issueId}`
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
