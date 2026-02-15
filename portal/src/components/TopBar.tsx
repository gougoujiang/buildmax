import type { Workspace } from "../lib/types"

interface TopBarProps {
  currentWorkspace: Workspace
  workspaces: Workspace[]
  onWorkspaceChange: (workspaceId: string) => void
  onNewWorkspace?: () => void
}

export function TopBar({
  currentWorkspace,
  workspaces,
  onWorkspaceChange,
  onNewWorkspace,
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
      <span className="topbar__profile" aria-label="Profile">
        Profile
      </span>
    </header>
  )
}
