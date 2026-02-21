import type { Route, Task } from "../lib/types"
import { navigate } from "../router"

const CHATS_LIMIT = 5

interface LeftSidebarProps {
  workspaceId: string
  route: Route
  currentWorkspace: { id: string; name: string }
  workspaces: { id: string; name: string }[]
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
  workspaceTasks: Task[]
}

function isAgentsActive(route: Route): boolean {
  return route.name === "agents" || route.name === "agentList"
}

function isTaskActive(route: Route, taskId: string): boolean {
  return route.name === "task" && route.taskId === taskId
}

export function LeftSidebar({
  workspaceId,
  route,
  currentWorkspace,
  workspaces,
  onWorkspaceChange,
  onNewWorkspace,
  workspaceTasks,
}: LeftSidebarProps) {
  const chats = workspaceTasks.slice(0, CHATS_LIMIT)
  const hasMoreChats = workspaceTasks.length > CHATS_LIMIT

  return (
    <aside className="sidebar" aria-label="Sidebar">
      <nav className="sidebar__nav" aria-label="Primary">
        <div className="sidebar__workspace">
          <span className="sidebar__workspace-label" title={currentWorkspace.name}>
            {currentWorkspace.name}
          </span>
          <div className="sidebar__workspace-controls">
            <select
              className="sidebar__workspace-select"
              value={currentWorkspace.id}
              onChange={(e) => onWorkspaceChange(e.target.value)}
              aria-label="Select workspace"
            >
              {workspaces.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </select>
            {onNewWorkspace && (
              <button
                type="button"
                className="sidebar__workspace-new"
                onClick={onNewWorkspace}
                aria-label="New workspace"
                title="New workspace"
              >
                +
              </button>
            )}
          </div>
        </div>

        <div className="sidebar__section">
          <button
            type="button"
            className={
              "sidebar__nav-item" +
              (route.name === "newChat" ? " sidebar__nav-item--active" : "")
            }
            onClick={() => navigate({ name: "newChat", workspaceId })}
          >
            New Chat
          </button>
          <button
            type="button"
            className={
              "sidebar__nav-item" +
              (isAgentsActive(route) ? " sidebar__nav-item--active" : "")
            }
            onClick={() => navigate({ name: "agents", workspaceId })}
          >
            Agents
          </button>

          <div className="sidebar__chats">
            <span className="sidebar__heading">Chats</span>
            <ul className="sidebar__list">
              {chats.map((task) => (
                <li key={task.id} className="sidebar__item">
                  <button
                    type="button"
                    className={
                      "sidebar__link" +
                      (isTaskActive(route, task.id) ? " sidebar__link--active" : "")
                    }
                    onClick={() =>
                      navigate({ name: "task", workspaceId, taskId: task.id })
                    }
                  >
                    <span className="sidebar__chat-title">
                      {task.title?.trim() || "New chat"}
                    </span>
                    <span className="sidebar__chat-meta">{task.timeLabel}</span>
                  </button>
                </li>
              ))}
            </ul>
            {hasMoreChats && (
              <button
                type="button"
                className="sidebar__chats-see-all"
                onClick={() => navigate({ name: "activity", workspaceId })}
              >
                See all
              </button>
            )}
          </div>
        </div>
      </nav>
    </aside>
  )
}
