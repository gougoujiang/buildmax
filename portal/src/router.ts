import { useState, useEffect } from "react"
import type { ProfileScope, Route } from "./lib/types"

/** Derive current scope from route (for data fetching and display). User is the top-level owner; profileId is the user id. */
export function getScope(route: Route): ProfileScope {
  return {
    profileId: route.profileId,
    conversationId: "conversationId" in route ? route.conversationId : undefined,
  }
}

/**
 * Path segment names used in the hash URL. Single source of truth for parseHash/buildHash.
 * Tier 1 uses conversation terminology; Tier 2 task detail uses `task`.
 */
export const SEGMENT = {
  new: "new",
  conversation: "conversation",
  conversations: "conversations",
  explore: "explore",
  agents: "agents",
} as const

/**
 * Parse window.location.hash into a typed Route.
 * Hash format: #<profileId> | #<profileId>/task/<taskId> | #<profileId>/conversations | ...
 * First segment = profileId (use as-is; if missing, "").
 */
export function parseHash(hash: string): Route {
  const raw = hash.replace(/^#\/?/, "")
  const parts = raw.split("/").filter(Boolean)

  const profileId = parts[0] ?? ""

  if (parts[1] === SEGMENT.new) {
    return { name: "newChat", profileId }
  }
  if (parts[1] === SEGMENT.conversations) {
    return { name: "chats", profileId }
  }
  if (parts[1] === SEGMENT.explore) {
    return { name: "explore", profileId }
  }
  if (parts[1] === SEGMENT.agents) {
    return { name: "agents", profileId }
  }
  if (parts[1] === SEGMENT.conversation && parts[2]) {
    return { name: "conversation", profileId, conversationId: parts[2] }
  }
  return { name: "home", profileId }
}

/** Convert a Route into a canonical hash string (includes leading #). */
export function buildHash(route: Route): string {
  switch (route.name) {
    case "home":
      return `#${route.profileId}`
    case "newChat":
      return `#${route.profileId}/${SEGMENT.new}`
    case "conversation":
      return `#${route.profileId}/${SEGMENT.conversation}/${route.conversationId}`
    case "chats":
      return `#${route.profileId}/${SEGMENT.conversations}`
    case "explore":
      return `#${route.profileId}/${SEGMENT.explore}`
    case "agents":
      return `#${route.profileId}/${SEGMENT.agents}`
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
