import type { LoginUser } from "../lib/api"

interface TopBarProps {
  user: LoginUser
  onLogout: () => void
}

export function TopBar({ user, onLogout }: TopBarProps) {
  return (
    <header className="topbar">
      <span className="topbar__brand">BuildMax</span>
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
