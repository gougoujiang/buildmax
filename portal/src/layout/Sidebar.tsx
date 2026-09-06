import { useState, useRef, useEffect } from "react"
import { cn } from "../lib/cn"
import type { Route } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { UserAvatar } from "../components/UserAvatar"
import SidebarExpandIcon from "../icons/sidebar-expand.svg?react"
import SidebarCollapseIcon from "../icons/sidebar-collapse.svg?react"
import NewChatIcon from "../icons/new-chat.svg?react"
import SettingsIcon from "../icons/settings.svg?react"
import HelpIcon from "../icons/help.svg?react"
import SignOutIcon from "../icons/sign-out.svg?react"
import IssueIcon from "../icons/issue.svg?react"
import WorkflowIcon from "../icons/workflow.svg?react"
import AgentsIcon from "../icons/agents.svg?react"
import ArtifactIcon from "../icons/artifact.svg?react"
import { CreateSpaceDialog } from "../components/CreateSpaceDialog"
import { useSpace } from "../contexts/SpaceContext"
import { useAdminAccess } from "../features/admin"

/** ASCII art for "BuildMax" (matches internal/tui/banner.go). */
const LOGO_ASCII = `
 ______        _ _     _ ______         _    _ 
(____  \\      (_) |   | |  ___ \\   /\\  \\ \\  / /
 ____)  )_   _ _| | _ | | | _ | | /  \\  \\ \\/ / 
|  __  (| | | | | |/ || | || || |/ /\\ \\  )  (  
| |__)  ) |_| | | ( (_| | || || | |__| |/ /\\ \\ 
|______/ \\____|_|_|\\____|_||_||_|______/_/  \\_\\
`.trim()

interface SidebarProps {
  route: Route
  user: LoginUser
  onLogout: () => void
}

function isAgentsActive(route: Route): boolean {
  return route.name === "agents" || route.name === "agent"
}

function isIssuesActive(route: Route): boolean {
  return route.name === "issues" || route.name === "issue"
}

function isWorkflowsActive(route: Route): boolean {
  return route.name === "workflows" || route.name === "workflow" || route.name === "workflowRun"
}

function isArtifactsActive(route: Route): boolean {
  return route.name === "artifacts" || route.name === "artifact"
}

export function Sidebar({
  route,
  user,
  onLogout,
}: SidebarProps) {
  const { spaces, currentSpace, currentSpaceId, loading: spacesLoading, setCurrentSpaceId } = useSpace()
  const { isAdmin: isSystemAdmin } = useAdminAccess()
  const showSpaceSwitcher = spaces.length > 1
  const personalSpaces = spaces.filter((space) => Boolean(space.personalForUserId))
  const sharedSpaces = spaces.filter((space) => !space.personalForUserId)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [createSpaceOpen, setCreateSpaceOpen] = useState(false)
  const userMenuRef = useRef<HTMLDivElement>(null)

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
          {!sidebarCollapsed ? (
            <div className="sidebar__space-switcher">
              <div className="sidebar__space-head">
                <label
                  className="sidebar__space-label"
                  htmlFor={showSpaceSwitcher ? "sidebar-space-select" : undefined}
                >
                  Space
                </label>
                <button
                  type="button"
                  className="sidebar__space-add"
                  onClick={() => setCreateSpaceOpen(true)}
                  aria-label="Create a new space"
                  title="Create a new space"
                >
                  +
                </button>
              </div>
              {showSpaceSwitcher ? (
                <select
                  id="sidebar-space-select"
                  className="sidebar__space-select"
                  value={currentSpaceId ?? ""}
                  onChange={(e) => setCurrentSpaceId(e.target.value)}
                  disabled={spacesLoading}
                >
                  {personalSpaces.length > 0 ? (
                    <optgroup label="Personal">
                      {personalSpaces.map((space) => (
                        <option key={space.id} value={space.id}>
                          {space.name}
                        </option>
                      ))}
                    </optgroup>
                  ) : null}
                  {sharedSpaces.length > 0 ? (
                    <optgroup label="Spaces">
                      {sharedSpaces.map((space) => (
                        <option key={space.id} value={space.id}>
                          {space.name}
                        </option>
                      ))}
                    </optgroup>
                  ) : null}
                </select>
              ) : (
                <div className="sidebar__space-display" aria-label="Current space">
                  {currentSpace?.name ?? "My Space"}
                </div>
              )}
            </div>
          ) : (
            <div className="sidebar__space-badge" title={currentSpace?.name ?? "Current space"}>
              {(currentSpace?.name ?? "S").slice(0, 1).toUpperCase()}
            </div>
          )}
          <button
            type="button"
            className={cn("sidebar__nav-item", route.name === "home" && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "home" })}
          >
            <NewChatIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Home</span>
          </button>
          <button
            type="button"
            className={cn("sidebar__nav-item", isIssuesActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "issues" })}
          >
            <IssueIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Issues</span>
          </button>
          <button
            type="button"
            className={cn("sidebar__nav-item", isWorkflowsActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "workflows" })}
          >
            <WorkflowIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Workflows</span>
          </button>
          <button
            type="button"
            className={cn("sidebar__nav-item", isAgentsActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "agents" })}
          >
            <AgentsIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Agents</span>
          </button>
          <button
            type="button"
            className={cn("sidebar__nav-item", isArtifactsActive(route) && "sidebar__nav-item--active")}
            onClick={() => navigate({ name: "artifacts" })}
          >
            <ArtifactIcon className="sidebar__nav-icon" aria-hidden />
            <span className="sidebar__nav-item-text">Artifacts</span>
          </button>
        </div>
      </nav>
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
            {!sidebarCollapsed && currentSpace ? (
              <>
                <div className="sidebar__user-menu-space" role="none">
                  <span className="sidebar__user-menu-space-label">Current space</span>
                  <span className="sidebar__user-menu-space-name">{currentSpace.name}</span>
                </div>
                <div className="sidebar__user-menu-divider" role="separator" />
              </>
            ) : null}
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => {
                setUserMenuOpen(false)
                navigate({ name: "account", section: "general" })
              }}
            >
              <span className="sidebar__user-menu-item-icon" aria-hidden>
                <SettingsIcon />
              </span>
              Account
            </button>
            <button
              type="button"
              className="sidebar__user-menu-item"
              role="menuitem"
              onClick={() => {
                setUserMenuOpen(false)
                navigate({ name: "space", section: "overview" })
              }}
            >
              <span className="sidebar__user-menu-item-icon" aria-hidden>
                <SettingsIcon />
              </span>
              Space
            </button>
            {/*
              Only for someone the server has confirmed may operate the
              deployment. Hiding it is presentation — /api/admin refuses either
              way — but an entry that leads to a forbidden screen is worse than
              no entry.
            */}
            {isSystemAdmin ? (
              <button
                type="button"
                className="sidebar__user-menu-item"
                role="menuitem"
                onClick={() => {
                  setUserMenuOpen(false)
                  navigate({ name: "admin", section: "overview" })
                }}
              >
                <span className="sidebar__user-menu-item-icon" aria-hidden>
                  <SettingsIcon />
                </span>
                Administration
              </button>
            ) : null}
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
      <CreateSpaceDialog open={createSpaceOpen} onClose={() => setCreateSpaceOpen(false)} />
    </aside>
  )
}
