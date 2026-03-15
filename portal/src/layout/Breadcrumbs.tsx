import type { Route, Conversation } from "../lib/types"
import { navigate } from "../router"

interface BreadcrumbsProps {
  route: Route
  conversations?: Conversation[]
}

export function Breadcrumbs({ route, conversations = [] }: BreadcrumbsProps) {
  const profileId = route.profileId
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "newChat") {
    crumbs = [{ label: "New Conversation", route: { name: "newChat", profileId } }]
  } else if (route.name === "chats") {
    crumbs = [{ label: "Conversations", route: { name: "chats", profileId } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Files", route: { name: "explore", profileId } }]
  } else if (route.name === "agents") {
    crumbs = [{ label: "Agents", route: { name: "agents", profileId } }]
  } else if (route.name === "conversation") {
    const conv = conversations.find((c) => c.id === route.conversationId)
    const convLabel = conv?.title?.trim() || conv?.timeLabel || "Conversation"
    crumbs = [
      { label: "Conversations", route: { name: "chats", profileId } },
      { label: convLabel, route },
    ]
  } else if (route.name === "task") {
    crumbs = [
      { label: "Conversations", route: { name: "chats", profileId } },
      { label: "Task", route },
    ]
  } else {
    crumbs = [{ label: "Home", route: { name: "home", profileId } }]
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
