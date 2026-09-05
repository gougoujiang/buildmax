import type { ReactNode } from "react"
import type { Conversation, Route } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { Sidebar } from "./Sidebar"
import { Breadcrumbs } from "./Breadcrumbs"
import { ThemeToggle } from "@buildmax/gui"
import { navigate } from "../router"

export interface LayoutProps {
  route: Route
  conversations: Conversation[]
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function Layout({
  route,
  conversations,
  user,
  onLogout,
  children,
}: LayoutProps) {
  return (
    <div className="shell">
      <div className="shell__body">
        <Sidebar
          route={route}
          user={user}
          onLogout={onLogout}
        />
        <main className="shell__main">
          <div className="shell__top">
            <Breadcrumbs route={route} conversations={conversations} />
            <div className="shell__top-actions">
              <MarketplaceButton active={route.name === "marketplace"} />
              <ThemeToggle />
            </div>
          </div>
          <div className="shell__content">{children}</div>
        </main>
      </div>
    </div>
  )
}

/** MarketplaceButton opens the deployment-wide plugin catalog. */
function MarketplaceButton({ active }: { active: boolean }) {
  return (
    <button
      type="button"
      className={`theme-toggle ${active ? "theme-toggle--active" : ""}`}
      onClick={() => navigate({ name: "marketplace" })}
      aria-label="Marketplace"
      aria-current={active ? "page" : undefined}
      title="Marketplace"
    >
      <StorefrontIcon className="theme-toggle__icon" />
    </button>
  )
}

function StorefrontIcon({ className }: { className?: string }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
    >
      <path d="M3 9.5 4.5 4h15L21 9.5" />
      <path d="M3 9.5a2.5 2.5 0 0 0 5 0 2.5 2.5 0 0 0 5 0 2.5 2.5 0 0 0 5 0 2.5 2.5 0 0 0 3 0" />
      <path d="M5 11v8a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-8" />
      <path d="M9 20v-5h6v5" />
    </svg>
  )
}
