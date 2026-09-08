import { useState, useEffect, useMemo } from "react"
import type { Route } from "./lib/types"

/**
 * Path segment names used in the hash URL. Single source of truth for parseHash/buildHash.
 *
 * A Space-owned route's canonical shape is `#/spaces/{space_id}/<resource>[/<id>]`,
 * one shared plural segment per resource family for both its collection and its
 * detail (`issues` / `issues/{id}`), per
 * docs/design/portal-navigation-and-space-context.md. Global routes (Account,
 * Admin, Marketplace, Help, Login) never carry a Space prefix. Artifact detail
 * is the one ID-resolved exception -- no Space in its path at all.
 */
export const SEGMENT = {
  login: "login",
  spaces: "spaces",
  chat: "chat",
  tasks: "tasks",
  files: "files",
  agents: "agents",
  account: "account",
  // The Space-settings resource word under a Space prefix, e.g. `#/spaces/{id}/settings`.
  space: "settings",
  admin: "admin",
  workflows: "workflows",
  workflowRuns: "workflow-runs",
  issues: "issues",
  artifacts: "artifacts",
  artifact: "artifact",
  marketplace: "marketplace",
  help: "help",
  // Pre-Space-prefix segments, recognized only to redirect old links into the
  // canonical shape above. Temporary: bounded to this migration, removed once
  // docs/design/portal-navigation-and-space-context.md is marked implemented.
  legacySpace: "space",
  legacySpaceSettings: "space-settings",
  legacyConversation: "conversation",
  legacyConversations: "conversations",
  legacyExplore: "explore",
  legacyAgent: "agent",
  legacyWorkflow: "workflow",
  legacyWorkflowRun: "workflow-run",
  legacyIssue: "issue",
  legacyTask: "task",
} as const

/**
 * Resolve a Space-scoped path (the segments after `#/spaces/{space_id}/`, or
 * their pre-migration equivalent) into a Route. `rest[0]` is always the
 * canonical plural resource word (`issues`, `agents`, ...); legacy callers
 * translate their own segment into that shape before calling this.
 */
function parseSpaceScopedRoute(spaceId: string, rest: string[]): Route {
  const [resource, id, sub] = rest
  switch (resource) {
    case SEGMENT.chat:
      return { name: "chat", spaceId, conversationId: id || undefined }
    case SEGMENT.issues:
      return id ? { name: "issue", spaceId, issueId: id } : { name: "issues", spaceId }
    case SEGMENT.agents:
      return id ? { name: "agent", spaceId, agentId: id } : { name: "agents", spaceId }
    case SEGMENT.workflows:
      return id ? { name: "workflow", spaceId, workflowId: id } : { name: "workflows", spaceId }
    case SEGMENT.workflowRuns:
      if (id) return { name: "workflowRun", spaceId, workflowRunId: id }
      break
    case SEGMENT.tasks:
      if (id) return { name: "task", spaceId, taskId: id }
      break
    case SEGMENT.files:
      return { name: "explore", spaceId }
    case SEGMENT.artifacts:
      return { name: "artifacts", spaceId }
    case SEGMENT.space:
      if (id === "members" && sub === "new") return { name: "space", spaceId, section: "memberNew" }
      if (id === "members") return { name: "space", spaceId, section: "members" }
      if (id === "plugins") return { name: "space", spaceId, section: "plugins" }
      if (id === "security") return { name: "space", spaceId, section: "security" }
      if (id === "secrets") return { name: "space", spaceId, section: "secrets" }
      if (id === "audit") return { name: "space", spaceId, section: "audit" }
      return { name: "space", spaceId, section: "overview" }
  }
  // An unrecognized or incomplete Space-scoped path. Until a not-found route
  // exists (a later slice of the navigation design), land on the Space's Chat
  // rather than guess further.
  return { name: "chat", spaceId }
}

/**
 * Parse window.location.hash into a typed Route. `currentSpaceId` resolves
 * routes that carry no Space id of their own: the bare `#/` entry point, and
 * every pre-migration flat hash (see SEGMENT's legacy* entries).
 */
export function parseHash(hash: string, currentSpaceId: string): Route {
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  // --- Global routes: never carry a Space prefix. ---
  if (parts[0] === SEGMENT.login) {
    return { name: "login" }
  }
  if (parts[0] === SEGMENT.account) {
    if (parts[1] === "usage") return { name: "account", section: "usage" }
    if (parts[1] === "webhook") return { name: "account", section: "webhook" }
    // Account's plugin catalog was a duplicate of Marketplace at a different
    // scope; kept as a redirect, not dropped, because the old address is what
    // any saved link points at. See docs/design/portal-data-and-plugin-surfaces.md.
    if (parts[1] === "plugins") return { name: "marketplace" }
    if (parts[1] === "invitations") return { name: "account", section: "invitations" }
    return { name: "account", section: "general" }
  }
  if (parts[0] === SEGMENT.admin) {
    if (parts[1] === "administrators") return { name: "admin", section: "administrators" }
    if (parts[1] === "accounts") return { name: "admin", section: "accounts", userId: parts[2] || undefined }
    if (parts[1] === "spaces") return { name: "admin", section: "spaces" }
    if (parts[1] === "models") return { name: "admin", section: "models" }
    if (parts[1] === "plugins") return { name: "admin", section: "plugins" }
    if (parts[1] === "audit") return { name: "admin", section: "audit" }
    return { name: "admin", section: "overview" }
  }
  if (parts[0] === SEGMENT.marketplace) {
    return { name: "marketplace" }
  }
  // #/help opens the manual's first page; #/help/<slug> opens one page.
  if (parts[0] === SEGMENT.help) {
    return { name: "help", slug: parts[1] }
  }
  // An artifact's address is its id alone -- no space in the path, matching the
  // API. See docs/design/unified-artifacts.md section 6.1.
  if (parts[0] === SEGMENT.artifact && parts[1]) {
    return { name: "artifact", artifactId: parts[1] }
  }

  // --- Canonical Space-prefixed routes. ---
  if (parts[0] === SEGMENT.spaces && parts[1]) {
    return parseSpaceScopedRoute(parts[1], parts.slice(2))
  }

  // --- Migration-bounded redirects for the pre-Space-prefix hash shapes.
  // Old links carry no Space id, so they resolve into whichever Space is
  // currently selected -- a wrong guess surfaces as the normal missing or
  // forbidden resource state, never stale data borrowed from another Space.
  if (parts[0] === SEGMENT.legacySpace && parts[1] === "artifacts") {
    // Artifacts left space settings for their own top-level area. Kept as a
    // redirect rather than dropped, because the old address is what any saved
    // link points at, and falling through would land on Overview silently.
    return parseSpaceScopedRoute(currentSpaceId, ["artifacts"])
  }
  if (parts[0] === SEGMENT.legacySpace) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.space, ...parts.slice(1)])
  }
  if (parts[0] === SEGMENT.legacySpaceSettings) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.space])
  }
  if (parts[0] === SEGMENT.legacyIssue && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.issues, parts[1]])
  }
  if (parts[0] === SEGMENT.issues) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.issues])
  }
  if (parts[0] === SEGMENT.legacyAgent && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.agents, parts[1]])
  }
  if (parts[0] === SEGMENT.agents) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.agents])
  }
  if (parts[0] === SEGMENT.legacyWorkflowRun && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.workflowRuns, parts[1]])
  }
  if (parts[0] === SEGMENT.legacyWorkflow && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.workflows, parts[1]])
  }
  if (parts[0] === SEGMENT.workflows) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.workflows])
  }
  if (parts[0] === SEGMENT.legacyTask && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.tasks, parts[1]])
  }
  if (parts[0] === SEGMENT.legacyExplore) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.files])
  }
  if (parts[0] === SEGMENT.artifacts) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.artifacts])
  }
  if (parts[0] === SEGMENT.legacyConversation && parts[1]) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.chat, parts[1]])
  }
  if (parts[0] === SEGMENT.legacyConversations) {
    return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.chat])
  }

  // Bare `#/` and anything else unrecognized: the Space's Chat.
  return parseSpaceScopedRoute(currentSpaceId, [SEGMENT.chat])
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "login":
      return `#/${SEGMENT.login}`
    case "chat":
      return route.conversationId
        ? `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.chat}/${route.conversationId}`
        : `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.chat}`
    case "task":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.tasks}/${route.taskId}`
    case "explore":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.files}`
    case "agents":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.agents}`
    case "agent":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.agents}/${route.agentId}`
    case "account":
      switch (route.section) {
        case "usage":
          return `#/${SEGMENT.account}/usage`
        case "webhook":
          return `#/${SEGMENT.account}/webhook`
        case "invitations":
          return `#/${SEGMENT.account}/invitations`
        case "general":
        default:
          return `#/${SEGMENT.account}`
      }
    case "space": {
      const prefix = `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.space}`
      switch (route.section) {
        case "members":
          return `${prefix}/members`
        case "memberNew":
          return `${prefix}/members/new`
        case "plugins":
          return `${prefix}/plugins`
        case "security":
          return `${prefix}/security`
        case "secrets":
          return `${prefix}/secrets`
        case "audit":
          return `${prefix}/audit`
        case "overview":
        default:
          return prefix
      }
    }
    case "admin":
      switch (route.section) {
        case "administrators":
          return `#/${SEGMENT.admin}/administrators`
        case "accounts":
          return route.userId
            ? `#/${SEGMENT.admin}/accounts/${route.userId}`
            : `#/${SEGMENT.admin}/accounts`
        case "spaces":
          return `#/${SEGMENT.admin}/spaces`
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
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.workflows}`
    case "workflow":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.workflows}/${route.workflowId}`
    case "workflowRun":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.workflowRuns}/${route.workflowRunId}`
    case "issues":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.issues}`
    case "issue":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.issues}/${route.issueId}`
    case "artifacts":
      return `#/${SEGMENT.spaces}/${route.spaceId}/${SEGMENT.artifacts}`
    case "artifact":
      return `#/${SEGMENT.artifact}/${route.artifactId}`
    case "marketplace":
      return `#/${SEGMENT.marketplace}`
    case "help":
      return route.slug ? `#/${SEGMENT.help}/${route.slug}` : `#/${SEGMENT.help}`
  }
}

/** Navigate to a Route by setting the hash. */
export function navigate(route: Route): void {
  window.location.hash = buildHash(route)
}

/**
 * React hook: returns the current Route and re-renders on hashchange.
 * `currentSpaceId` resolves the bare `#/` entry point and any legacy hash
 * that carries no Space id of its own -- pass `""` while it is still
 * unresolved (e.g. the account's Spaces have not loaded yet); every
 * Space-scoped Route's `spaceId` will read as `""` until a real one is
 * available.
 */
export function useHashRoute(currentSpaceId: string): Route {
  const [hash, setHash] = useState<string>(() => window.location.hash)

  useEffect(() => {
    function onHashChange() {
      setHash(window.location.hash)
    }
    window.addEventListener("hashchange", onHashChange)
    return () => window.removeEventListener("hashchange", onHashChange)
  }, [])

  return useMemo(() => parseHash(hash, currentSpaceId), [hash, currentSpaceId])
}
