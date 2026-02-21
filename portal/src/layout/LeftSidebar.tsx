import type { Route, Chat } from "../lib/types"
import { navigate } from "../router"

const CHATS_LIMIT = 5

interface LeftSidebarProps {
  workspaceId: string
  route: Route
  currentWorkspace: { id: string; name: string }
  workspaces: { id: string; name: string }[]
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
  workspaceChats: Chat[]
}

function isAgentsActive(route: Route): boolean {
  return route.name === "agents"
}

function isChatActive(route: Route, chatId: string): boolean {
  return route.name === "chat" && route.chatId === chatId
}

export function LeftSidebar({
  workspaceId,
  route,
  currentWorkspace,
  workspaces,
  onWorkspaceChange,
  onNewWorkspace,
  workspaceChats,
}: LeftSidebarProps) {
  const chats = workspaceChats.slice(0, CHATS_LIMIT)
  const hasMoreChats = workspaceChats.length > CHATS_LIMIT

  return (
    <aside className="sidebar" aria-label="Sidebar">
      <nav className="sidebar__nav" aria-label="Primary">
        <div className="sidebar__workspace">
          <span className="sidebar__workspace-label" title="Workspace">
            Workspace
          </span>
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
          <div className="sidebar__chats">
            <span className="sidebar__heading">Recent</span>
            <ul className="sidebar__list">
              {chats.map((chat) => (
                <li key={chat.id} className="sidebar__item">
                  <button
                    type="button"
                    className={
                      "sidebar__link" +
                      (isChatActive(route, chat.id) ? " sidebar__link--active" : "")
                    }
                    onClick={() =>
                      navigate({ name: "chat", workspaceId, chatId: chat.id })
                    }
                  >
                    <span className="sidebar__chat-title">
                      {chat.title?.trim() || "New chat"}
                    </span>
                    <span className="sidebar__chat-meta">{chat.timeLabel}</span>
                  </button>
                </li>
              ))}
            </ul>
            {hasMoreChats && (
              <button
                type="button"
                className="sidebar__chats-see-all"
                onClick={() => navigate({ name: "chats", workspaceId })}
              >
                See all
              </button>
            )}
          </div>
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
        </div>
      </nav>
    </aside>
  )
}
