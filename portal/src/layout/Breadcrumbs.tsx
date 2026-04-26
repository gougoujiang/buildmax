import type { Route, Conversation } from "../lib/types"
import { navigate } from "../router"

interface BreadcrumbsProps {
  route: Route
  conversations?: Conversation[]
}

export function Breadcrumbs({ route, conversations = [] }: BreadcrumbsProps) {
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "conversations") {
    crumbs = [{ label: "Conversations", route: { name: "conversations" } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Files", route: { name: "explore" } }]
  } else if (route.name === "agents") {
    crumbs = [{ label: "Agents", route: { name: "agents" } }]
  } else if (route.name === "workflows") {
    crumbs = [{ label: "Workflows", route: { name: "workflows" } }]
  } else if (route.name === "workflow") {
    crumbs = [
      { label: "Workflows", route: { name: "workflows" } },
      { label: route.workflowId, route },
    ]
  } else if (route.name === "workflowRun") {
    crumbs = [
      { label: "Workflows", route: { name: "workflows" } },
      { label: route.workflowRunId, route },
    ]
  } else if (route.name === "issues") {
    crumbs = [{ label: "Issues", route: { name: "issues" } }]
  } else if (route.name === "issue") {
    crumbs = [
      { label: "Issues", route: { name: "issues" } },
      { label: route.issueId, route },
    ]
  } else if (route.name === "conversation") {
    const conv = conversations.find((c) => c.id === route.conversationId)
    const convLabel = conv?.title?.trim() || conv?.timeLabel || "Conversation"
    crumbs = [
      { label: "Home", route: { name: "home" } },
      { label: convLabel, route },
    ]
  } else {
    crumbs = [{ label: "Home", route: { name: "home" } }]
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
