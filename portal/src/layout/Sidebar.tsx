import { useState, useRef, useEffect } from "react"
import { createPortal } from "react-dom"
import { cn } from "../lib/cn"
import type { Route, Chat } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { UserAvatar, AgentAvatar } from "../components/UserAvatar"
import SidebarExpandIcon from "../icons/sidebar-expand.svg?react"
import SidebarCollapseIcon from "../icons/sidebar-collapse.svg?react"
import NewChatIcon from "../icons/new-chat.svg?react"
import { SettingsModal } from "../components/SettingsModal"

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

const CHATS_LIMIT = 5

interface SidebarProps {
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

export function Sidebar({
  workspaceId,
  route,
  currentWorkspace,
  workspaces,
  onWorkspaceChange,
  onNewWorkspace,
  workspaceChats,
  user,
  onLogout,
}: SidebarProps) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [chatsCollapsed, setChatsCollapsed] = useState(false)
  const [chatsPopupOpen, setChatsPopupOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [settingsModalOpen, setSettingsModalOpen] = useState(false)
  const userMenuRef = useRef<HTMLDivElement>(null)
  const chatsTriggerRef = useRef<HTMLDivElement>(null)
  const chatsCloseTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const chats = workspaceChats.slice(0, CHATS_LIMIT)
  const hasMoreChats = workspaceChats.length > CHATS_LIMIT

  function openChatsPopup() {
    if (chatsCloseTimeoutRef.current) {
      clearTimeout(chatsCloseTimeoutRef.current)
      chatsCloseTimeoutRef.current = null
    }
    setChatsPopupOpen(true)
  }

  function scheduleCloseChatsPopup() {
    chatsCloseTimeoutRef.current = setTimeout(() => setChatsPopupOpen(false), 150)
  }

  useEffect(() => {
    if (!userMenuOpen) return
    function handleClickOutside(e: MouseEvent) {
      if (userMenuRef.current && !userMenuRef.current.contains(e.target as Node)) {
        setUserMenuOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [userMenuOpen])

  return (
    <aside
      className={cn("sidebar", sidebarCollapsed && "sidebar--collapsed")}
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
        <div className="sidebar__section">
          <button
            type="button"
            className={cn("sidebar__nav-item", route.name === "newChat" && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "newChat", workspaceId })}
          >
            <NewChatIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">New Chat</span>
          </button>
          <div
            className="sidebar__chats"
            ref={chatsTriggerRef}
            onMouseEnter={sidebarCollapsed ? openChatsPopup : undefined}
            onMouseLeave={sidebarCollapsed ? scheduleCloseChatsPopup : undefined}
          >
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
            {!sidebarCollapsed && (
              <div
                id="sidebar-chats-list"
                className="sidebar__chats-list"
                hidden={chatsCollapsed}
              >
                <ul className="sidebar__list">
                  {chats.map((chat) => (
                    <li key={chat.id} className="sidebar__item">
                      <button
                        type="button"
                        className={cn("sidebar__link", isChatActive(route, chat.id) && "sidebar__link--active")}
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
            )}
          </div>
          {sidebarCollapsed &&
            chatsPopupOpen &&
            chatsTriggerRef.current &&
            createPortal(
              <div
                className="sidebar__chats-list sidebar__chats-list--portal"
                onMouseEnter={openChatsPopup}
                onMouseLeave={scheduleCloseChatsPopup}
                style={{
                  position: "fixed",
                  left: chatsTriggerRef.current.getBoundingClientRect().right + 4,
                  top: chatsTriggerRef.current.getBoundingClientRect().top,
                  zIndex: 1000,
                }}
              >
                <ul className="sidebar__list">
                  {chats.map((chat) => (
                    <li key={chat.id} className="sidebar__item">
                      <button
                        type="button"
                        className={cn("sidebar__link", isChatActive(route, chat.id) && "sidebar__link--active")}
                        onClick={() => {
                          setChatsPopupOpen(false)
                          navigate({ name: "chat", workspaceId, chatId: chat.id })
                        }}
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
                    onClick={() => {
                      setChatsPopupOpen(false)
                      navigate({ name: "chats", workspaceId })
                    }}
                  >
                    See all
                  </button>
                )}
              </div>,
              document.body
            )}
          <button
            type="button"
            className={cn("sidebar__nav-item", isAgentsActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "agents", workspaceId })}
          >
            <AgentAvatar size="sm" />
            <span className="sidebar__nav-item-text">Agents</span>
          </button>
        </div>
      </nav>
      <div className="sidebar__workspace-wrap">
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
      </div>
      <div className="sidebar__footer" aria-label="User" ref={userMenuRef}>
        <button
          type="button"
          className="sidebar__user-trigger"
          onClick={() => setUserMenuOpen((open) => !open)}
          aria-expanded={userMenuOpen}
          aria-haspopup="menu"
          aria-label="User menu"
        >
          <UserAvatar user={user} size="sm" />
          <span className="sidebar__user-name">{user.name || user.email}</span>
        </button>
        {userMenuOpen && (
          <div className="sidebar__user-menu" role="menu">
            <div className="sidebar__user-menu-header" role="none">
              <UserAvatar user={user} size="md" />
              <div className="sidebar__user-menu-header-text">
                {user.name ? (
                  <span className="sidebar__user-menu-name">{user.name}</span>
                ) : null}
                <span className="sidebar__user-menu-email">{user.email}</span>
              </div>
            </div>
            <div className="sidebar__user-menu-divider" role="separator" />
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => {
                setUserMenuOpen(false)
                setSettingsModalOpen(true)
              }}
            >
              Settings
            </button>
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => setUserMenuOpen(false)}
            >
              Help
            </button>
            <div className="sidebar__user-menu-divider" role="separator" />
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => {
                setUserMenuOpen(false)
                onLogout()
              }}
            >
              Sign Out
            </button>
          </div>
        )}
      </div>
      <SettingsModal open={settingsModalOpen} onClose={() => setSettingsModalOpen(false)} />
    </aside>
  )
}
