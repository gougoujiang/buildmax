import type { ReactNode } from "react"
import type { Conversation, Route } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { Sidebar } from "./Sidebar"
import { Breadcrumbs } from "./Breadcrumbs"
import { ThemeToggle } from "@buildmax/gui"

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
          conversations={conversations}
          user={user}
          onLogout={onLogout}
        />
        <main className="shell__main">
          <div className="shell__top">
            <Breadcrumbs route={route} conversations={conversations} />
            <ThemeToggle />
          </div>
          <div className="shell__content">{children}</div>
        </main>
      </div>
    </div>
  )
}
