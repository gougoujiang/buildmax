import type { Workspace } from "../lib/types"
import type { LoginUser } from "../lib/api"

interface TopBarProps {
  currentWorkspace: Workspace
  workspaces: Workspace[]
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
  user: LoginUser
  onLogout: () => void
}

export function TopBar({
  currentWorkspace,
  workspaces,
  onWorkspaceChange,
  onNewWorkspace,
  user,
  onLogout,
}: TopBarProps) {
  return (
    <header className="topbar">
      <span className="topbar__brand">BuildMax</span>
      <div className="topbar__workspace-wrap">
        <label htmlFor="workspace-select" className="topbar__workspace-label">
          Workspace:
        </label>
        <select
          id="workspace-select"
          className="topbar__workspace-select"
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
            className="topbar__workspace-new"
            onClick={onNewWorkspace}
            aria-label="New workspace"
            title="New workspace"
          >
            + New
          </button>
        )}
      </div>
      <div className="topbar__profile" aria-label="Profile">
        <span className="topbar__profile-name">{user.name || user.email}</span>
        <button
          type="button"
          className="topbar__logout"
          onClick={onLogout}
          aria-label="Log out"
        >
          Logout
        </button>
      </div>
    </header>
  )
}
