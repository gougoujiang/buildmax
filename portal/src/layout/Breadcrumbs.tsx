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
    crumbs = [{ label: "Workspace Files", route: { name: "explore" } }]
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
        case "plugins":
          return "Plugins"
        case "security":
          return "Security"
        case "secrets":
          return "Secrets"
        case "audit":
          return "Audit"
        case "overview":
        default:
          return "Overview"
      }
    })()
    crumbs = [
      { label: "Space settings", route: { name: "space", section: "overview" } },
      { label: sectionLabel, route },
    ]
  } else if (route.name === "admin") {
    const sectionLabel = (() => {
      switch (route.section) {
        case "administrators":
          return "Administrators"
        case "accounts":
          return "Accounts"
        case "spaces":
          return "Spaces"
        case "models":
          return "Models"
        case "plugins":
          return "Plugins"
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
      { label: entityLabels[route.workflowId] ?? "Workflow", route },
    ]
  } else if (route.name === "workflowRun") {
    crumbs = [
      { label: "Workflows", route: { name: "workflows" } },
      { label: entityLabels[route.workflowRunId] ?? "Workflow Run", route },
    ]
  } else if (route.name === "issues") {
    crumbs = [{ label: "Issues", route: { name: "issues" } }]
  } else if (route.name === "issue") {
    crumbs = [
      { label: "Issues", route: { name: "issues" } },
      { label: entityLabels[route.issueId] ?? "Issue", route },
    ]
  } else if (route.name === "artifacts") {
    crumbs = [{ label: "Artifacts", route: { name: "artifacts" } }]
  } else if (route.name === "artifact") {
    crumbs = [
      { label: "Artifacts", route: { name: "artifacts" } },
      { label: entityLabels[route.artifactId] ?? "Artifact", route },
    ]
  } else if (route.name === "marketplace") {
    crumbs = [{ label: "Marketplace", route: { name: "marketplace" } }]
  } else if (route.name === "task") {
    // A task's parents (agent / issue / conversation) are not in the route, so
    // the detail page publishes the trail; fall back until it loads.
    crumbs = breadcrumbTrails[route.taskId] ?? [
      { label: "Chat", route: { name: "home" } },
      { label: "Task", route },
    ]
  } else if (route.name === "conversation") {
    const conv = conversations.find((c) => c.id === route.conversationId)
    const convLabel = conv?.title?.trim() || conv?.timeLabel || "Conversation"
    crumbs = [
      { label: "Chat", route: { name: "home" } },
      { label: convLabel, route },
    ]
  } else {
    crumbs = [{ label: "Chat", route: { name: "home" } }]
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
