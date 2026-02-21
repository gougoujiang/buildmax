import { useState } from "react"
import type { Route, Chat } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import SidebarExpandIcon from "../icons/sidebar-expand.svg?react"
import SidebarCollapseIcon from "../icons/sidebar-collapse.svg?react"
import NewChatIcon from "../icons/new-chat.svg?react"

/** ASCII art for "BuildMax" (matches internal/tui/banner.go). */
const LOGO_ASCII = `
 ______        _ _     _ ______         _    _ 
(____  \\      (_) |   | |  ___ \\   /\\  \\ \\  / /
 ____)  )_   _ _| | _ | | | _ | | /  \\  \\ \\/ / 
|  __  (| | | | | |/ || | || || |/ /\\ \\  )  (  
| |__)  ) |_| | | ( (_| | || || | |__| |/ /\\ \\ 
|______/ \\____|_|_|\\____|_||_||_|______/_/  \\_\\
`.trim()

/** First letter "B" extracted from LOGO_ASCII for collapsed sidebar. */
const ASCII_B = LOGO_ASCII.split("\n").map((line) => line.slice(0, 7)).join("\n")

import RecentIcon from "../icons/recent.svg?react"
import AgentsIcon from "../icons/agents.svg?react"

const CHATS_LIMIT = 5

interface LeftSidebarProps {
  workspaceId: string
  route: Route
  currentWorkspace: { id: string; name: string }
  workspaces: { id: string; name: string }[]
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
  workspaceChats: Chat[]
  user: LoginUser
  onLogout: () => void
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
  user,
  onLogout,
}: LeftSidebarProps) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [chatsCollapsed, setChatsCollapsed] = useState(false)
  const chats = workspaceChats.slice(0, CHATS_LIMIT)
  const hasMoreChats = workspaceChats.length > CHATS_LIMIT

  return (
    <aside
      className={"sidebar" + (sidebarCollapsed ? " sidebar--collapsed" : "")}
      aria-label="Sidebar"
    >
      <div className="sidebar__header">
        {sidebarCollapsed ? (
          <button
            type="button"
            className="sidebar__logo-expand"
            onClick={() => setSidebarCollapsed(false)}
            aria-label="Expand sidebar"
            title="Expand sidebar"
          >
            <pre className="sidebar__logo-ascii-collapsed" aria-hidden>
              {ASCII_B}
            </pre>
            <SidebarExpandIcon className="sidebar__logo-expand-icon" aria-hidden />
          </button>
        ) : (
          <>
            <pre className="sidebar__logo-ascii" aria-hidden>
              {LOGO_ASCII}
            </pre>
            <button
              type="button"
              className="sidebar__collapse-btn"
              onClick={() => setSidebarCollapsed(true)}
              aria-label="Collapse sidebar"
              title="Collapse sidebar"
            >
              <SidebarCollapseIcon className="sidebar__collapse-btn-icon" aria-hidden />
            </button>
          </>
        )}
      </div>
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
            <NewChatIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">New Chat</span>
          </button>
          <div className="sidebar__chats">
            <button
              type="button"
              className="sidebar__chats-toggle"
              onClick={() => setChatsCollapsed((c) => !c)}
              aria-expanded={!chatsCollapsed}
              aria-controls="sidebar-chats-list"
            >
              <RecentIcon className="sidebar__nav-icon" aria-hidden />
              <span className="sidebar__heading">Recent Chats</span>
              <span className="sidebar__chats-chevron" aria-hidden>
                {chatsCollapsed ? "▶" : "▼"}
              </span>
            </button>
            <div id="sidebar-chats-list" className="sidebar__chats-list" hidden={chatsCollapsed}>
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
          </div>
          <button
            type="button"
            className={
              "sidebar__nav-item" +
              (isAgentsActive(route) ? " sidebar__nav-item--active" : "")
            }
            onClick={() => navigate({ name: "agents", workspaceId })}
          >
            <AgentsIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Agents</span>
          </button>
        </div>
      </nav>
      <div className="sidebar__footer" aria-label="User">
        <span className="sidebar__user-name" title={user.email}>
          {user.name || user.email}
        </span>
        <button
          type="button"
          className="sidebar__logout"
          onClick={onLogout}
          aria-label="Log out"
        >
          Logout
        </button>
      </div>
    </aside>
  )
}
