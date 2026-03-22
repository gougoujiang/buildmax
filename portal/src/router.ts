import { useState, useEffect } from "react"
import type { Route } from "./lib/types"

/**
 * Path segment names used in the hash URL. Single source of truth for parseHash/buildHash.
 */
export const SEGMENT = {
  login: "login",
  signup: "signup",
  conversation: "conversation",
  conversations: "conversations",
  explore: "explore",
  agents: "agents",
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
  if (parts[0] === SEGMENT.signup) {
    return { name: "signup" }
  }
  if (parts[0] === SEGMENT.conversations) {
    return { name: "chats" }
  }
  if (parts[0] === SEGMENT.explore) {
    return { name: "explore" }
  }
  if (parts[0] === SEGMENT.agents) {
    return { name: "agents" }
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
    case "signup":
      return `#/${SEGMENT.signup}`
    case "conversation":
      return `#/${SEGMENT.conversation}/${route.conversationId}`
    case "chats":
      return `#/${SEGMENT.conversations}`
    case "explore":
      return `#/${SEGMENT.explore}`
    case "agents":
      return `#/${SEGMENT.agents}`
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
