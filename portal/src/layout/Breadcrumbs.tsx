import type { Route, Conversation } from "../lib/types"
import { navigate } from "../router"
import { useApp } from "../contexts/AppContext"

interface BreadcrumbsProps {
  route: Route
  conversations?: Conversation[]
}

export function Breadcrumbs({ route, conversations = [] }: BreadcrumbsProps) {
  const { entityLabels, breadcrumbTrails } = useApp()
  let crumbs: { label: string; route: Route }[] = []

  if (route.name === "conversations") {
    crumbs = [{ label: "Conversations", route: { name: "conversations" } }]
  } else if (route.name === "explore") {
    crumbs = [{ label: "Files", route: { name: "explore" } }]
  } else if (route.name === "agents") {
    crumbs = [{ label: "Agents", route: { name: "agents" } }]
  } else if (route.name === "agent") {
    crumbs = [
      { label: "Agents", route: { name: "agents" } },
      { label: entityLabels[route.agentId] ?? "Agent", route },
    ]
  } else if (route.name === "account") {
    const sectionLabel = (() => {
      switch (route.section) {
        case "usage":
          return "Usage"
        case "webhook":
          return "Webhook Keys"
        case "general":
        default:
          return "General"
      }
    })()
    crumbs = [
      { label: "Account", route: { name: "account", section: "general" } },
      { label: sectionLabel, route },
    ]
  } else if (route.name === "space") {
    const sectionLabel = (() => {
      switch (route.section) {
        case "members":
          return "Members"
        case "memberNew":
          return "Invite Member"
        case "overview":
        default:
          return "Overview"
      }
    })()
    crumbs = [
      { label: "Space", route: { name: "space", section: "overview" } },
      { label: sectionLabel, route },
    ]
  } else if (route.name === "admin") {
    const sectionLabel = (() => {
      switch (route.section) {
        case "accounts":
          return "Accounts"
        case "teams":
          return "Spaces"
        case "models":
          return "Models"
        case "audit":
          return "Audit"
        case "overview":
        default:
          return "Overview"
      }
    })()
    crumbs = [
      { label: "Administration", route: { name: "admin", section: "overview" } },
      { label: sectionLabel, route },
    ]
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
  } else if (route.name === "artifacts") {
    crumbs = [{ label: "Artifacts", route: { name: "artifacts" } }]
  } else if (route.name === "artifact") {
    crumbs = [
      { label: "Artifacts", route: { name: "artifacts" } },
      { label: route.artifactId, route },
    ]
  } else if (route.name === "task") {
    // A task's parents (agent / issue / conversation) are not in the route, so
    // the detail page publishes the trail; fall back until it loads.
    crumbs = breadcrumbTrails[route.taskId] ?? [
      { label: "Home", route: { name: "home" } },
      { label: "Task", route },
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
