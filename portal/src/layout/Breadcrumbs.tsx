import type { Route, Chat } from "../lib/types"
import { navigate } from "../router"

interface BreadcrumbsProps {
  route: Route
  workspaceChats?: Chat[]
}

export function Breadcrumbs({ route, workspaceChats = [] }: BreadcrumbsProps) {
  const workspaceId = route.workspaceId
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "newChat") {
    crumbs = [{ label: "New Chat", route: { name: "newChat", workspaceId } }]
  } else if (route.name === "chats") {
    crumbs = [{ label: "Chats", route: { name: "chats", workspaceId } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Files", route: { name: "explore", workspaceId } }]
  } else if (route.name === "agents") {
    crumbs = [{ label: "Agents", route: { name: "agents", workspaceId } }]
  } else if (route.name === "chat") {
    const chat = workspaceChats.find((c) => c.id === route.chatId)
    const chatTitle = chat?.title?.trim() || "Chat"
    crumbs = [
      { label: "Chats", route: { name: "chats", workspaceId } },
      { label: chatTitle, route },
    ]
  } else if (route.name === "conversation") {
    crumbs = [
      { label: "New Chat", route: { name: "newChat", workspaceId } },
      { label: "Conversation", route },
    ]
  } else {
    crumbs = [{ label: "Home", route: { name: "workspace", workspaceId } }]
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
