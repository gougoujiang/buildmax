import { useState, useRef, useEffect } from "react"
import { createPortal } from "react-dom"
import { RecentList } from "@buildmax/gui"
import { cn } from "../lib/cn"
import type { Route, Conversation } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { UserAvatar, AgentAvatar } from "../components/UserAvatar"
import SidebarExpandIcon from "../icons/sidebar-expand.svg?react"
import SidebarCollapseIcon from "../icons/sidebar-collapse.svg?react"
import NewChatIcon from "../icons/new-chat.svg?react"
import SettingsIcon from "../icons/settings.svg?react"
import HelpIcon from "../icons/help.svg?react"
import SignOutIcon from "../icons/sign-out.svg?react"
import { SettingsModal } from "../components/SettingsModal"
import { WorkspaceSelect } from "../components/WorkspaceSelect"

/** ASCII art for "BuildMax" (matches internal/tui/banner.go). */
const LOGO_ASCII = `
 ______        _ _     _ ______         _    _ 
(____  \\      (_) |   | |  ___ \\   /\\  \\ \\  / /
 ____)  )_   _ _| | _ | | | _ | | /  \\  \\ \\/ / 
|  __  (| | | | | |/ || | || || |/ /\\ \\  )  (  
| |__)  ) |_| | | ( (_| | || || | |__| |/ /\\ \\ 
|______/ \\____|_|_|\\____|_||_||_|______/_/  \\_\\
`.trim()

import RecentIcon from "../icons/recent.svg?react"

const CHATS_LIMIT = 5

interface SidebarProps {
  profileId: string
  route: Route
  currentProfile: { id: string; name: string }
  profiles: { id: string; name: string }[]
  onProfileChange: (profileId: string) => void
  workspaceConversations: Conversation[]
  user: LoginUser
  onLogout: () => void
}

function isAgentsActive(route: Route): boolean {
  return route.name === "agents"
}

export function Sidebar({
  profileId,
  route,
  currentProfile,
  profiles,
  onProfileChange,
  workspaceConversations,
  user,
  onLogout,
}: SidebarProps) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [conversationsCollapsed, setConversationsCollapsed] = useState(false)
  const [conversationsPopupOpen, setConversationsPopupOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [settingsModalOpen, setSettingsModalOpen] = useState(false)
  const userMenuRef = useRef<HTMLDivElement>(null)
  const conversationsTriggerRef = useRef<HTMLDivElement>(null)
  const conversationsCloseTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const conversations = workspaceConversations.slice(0, CHATS_LIMIT)
  const hasMoreConversations = workspaceConversations.length > CHATS_LIMIT

  function openConversationsPopup() {
    if (conversationsCloseTimeoutRef.current) {
      clearTimeout(conversationsCloseTimeoutRef.current)
      conversationsCloseTimeoutRef.current = null
    }
    setConversationsPopupOpen(true)
  }

  function scheduleCloseConversationsPopup() {
    conversationsCloseTimeoutRef.current = setTimeout(() => setConversationsPopupOpen(false), 150)
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
            onClick={() => navigate({ name: "newChat", profileId })}
          >
            <NewChatIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">New Conversation</span>
          </button>
          <div
            className="sidebar__chats"
            ref={conversationsTriggerRef}
            onMouseEnter={sidebarCollapsed ? openConversationsPopup : undefined}
            onMouseLeave={sidebarCollapsed ? scheduleCloseConversationsPopup : undefined}
          >
            <button
              type="button"
              className="sidebar__chats-toggle"
              onClick={() => setConversationsCollapsed((c) => !c)}
              aria-expanded={!conversationsCollapsed}
              aria-controls="sidebar-conversations-list"
            >
              <RecentIcon className="sidebar__nav-icon" aria-hidden />
              <span className="sidebar__heading">Recent</span>
              <span className="sidebar__chats-chevron" aria-hidden>
                {conversationsCollapsed ? "▶" : "▼"}
              </span>
            </button>
            {!sidebarCollapsed && (
              <div
                id="sidebar-conversations-list"
                className="sidebar__chats-list"
                hidden={conversationsCollapsed}
              >
                <RecentList
                  items={conversations.map((conv) => ({
                    id: conv.id,
                    title: conv.title?.trim() || "Conversation",
                    meta: conv.timeLabel,
                  }))}
                  activeId={route.name === "conversation" ? route.conversationId : null}
                  onSelect={(conversationId) =>
                    navigate({ name: "conversation", profileId, conversationId })
                  }
                  moreActionLabel={hasMoreConversations ? "See all" : undefined}
                  onMoreAction={hasMoreConversations ? () => navigate({ name: "chats", profileId }) : undefined}
                  moreActionClassName="sidebar__chats-see-all"
                />
              </div>
            )}
          </div>
          {sidebarCollapsed &&
            conversationsPopupOpen &&
            conversationsTriggerRef.current &&
            createPortal(
              <div
                className="sidebar__chats-list sidebar__chats-list--portal"
                onMouseEnter={openConversationsPopup}
                onMouseLeave={scheduleCloseConversationsPopup}
                style={{
                  position: "fixed",
                  left: conversationsTriggerRef.current.getBoundingClientRect().right + 4,
                  top: conversationsTriggerRef.current.getBoundingClientRect().top,
                  zIndex: 1000,
                }}
              >
                <RecentList
                  items={conversations.map((conv) => ({
                    id: conv.id,
                    title: conv.title?.trim() || "Conversation",
                    meta: conv.timeLabel,
                  }))}
                  activeId={route.name === "conversation" ? route.conversationId : null}
                  onSelect={(conversationId) => {
                    setConversationsPopupOpen(false)
                    navigate({ name: "conversation", profileId, conversationId })
                  }}
                  moreActionLabel={hasMoreConversations ? "See all" : undefined}
                  onMoreAction={hasMoreConversations ? () => {
                    setConversationsPopupOpen(false)
                    navigate({ name: "chats", profileId })
                  } : undefined}
                  moreActionClassName="sidebar__chats-see-all"
                />
              </div>,
              document.body
            )}
          <button
            type="button"
            className={cn("sidebar__nav-item", isAgentsActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "agents", profileId })}
          >
            <AgentAvatar size="sm" />
            <span className="sidebar__nav-item-text">Agents</span>
          </button>
        </div>
      </nav>
      <div className="sidebar__workspace-wrap">
        <div className="sidebar__workspace">
          <span className="sidebar__workspace-label" title="Profile">
            Profile
          </span>
          <WorkspaceSelect
            value={currentProfile.id}
            options={profiles}
            onChange={onProfileChange}
            ariaLabel="Select profile"
          />
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
          <span className="sidebar__user-name">
            {user.name?.trim() || (user.email ? user.email.split("@")[0] : "")}
          </span>
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
              <span className="sidebar__user-menu-item-icon" aria-hidden>
                <SettingsIcon />
              </span>
              Settings
            </button>
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => setUserMenuOpen(false)}
            >
              <span className="sidebar__user-menu-item-icon" aria-hidden>
                <HelpIcon />
              </span>
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
              <span className="sidebar__user-menu-item-icon" aria-hidden>
                <SignOutIcon />
              </span>
              Sign Out
            </button>
          </div>
        )}
      </div>
      <SettingsModal open={settingsModalOpen} onClose={() => setSettingsModalOpen(false)} />
    </aside>
  )
}
