interface TopBarProps {
  workspaceName: string
}

export function TopBar({ workspaceName }: TopBarProps) {
  return (
    <header className="topbar">
      <span className="topbar__brand">BuildMax Portal</span>
      <span className="topbar__workspace">Workspace: {workspaceName}</span>
      <span className="topbar__profile" aria-label="Profile">
        Profile
      </span>
    </header>
  )
}
