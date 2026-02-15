interface HeaderProps {
  workspaceName?: string
}

export function Header({ workspaceName = "Default" }: HeaderProps) {
  return (
    <header className="landing-header">
      <span className="landing-header__brand">BuildMax Portal</span>
      <span className="landing-header__workspace">Workspace: {workspaceName}</span>
      <span className="landing-header__profile" aria-label="Profile">⚙️ Profile</span>
    </header>
  )
}
