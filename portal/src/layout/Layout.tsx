import type { ReactNode } from "react"
import type { Conversation, Profile, Route } from "../lib/types"
import type { LoginUser } from "../lib/api"
import { navigate } from "../router"
import { Sidebar } from "./Sidebar"
import { Breadcrumbs } from "./Breadcrumbs"
import { ThemeToggle } from "@buildmax/gui"

export interface LayoutProps {
  route: Route
  currentProfile: Profile
  profiles: { id: string; name: string }[]
  profileConversations: Conversation[]
  user: LoginUser
  onLogout: () => void
  children: ReactNode
}

export function Layout({
  route,
  currentProfile,
  profiles,
  profileConversations,
  user,
  onLogout,
  children,
}: LayoutProps) {
  return (
    <div className="shell">
      <div className="shell__body">
        <Sidebar
          profileId={route.profileId}
          route={route}
          currentProfile={currentProfile}
          profiles={profiles}
          onProfileChange={(profileId) => navigate({ name: "home", profileId })}
          workspaceConversations={profileConversations}
          user={user}
          onLogout={onLogout}
        />
        <main className="shell__main">
          <div className="shell__top">
            <Breadcrumbs route={route} workspaceConversations={profileConversations} />
            <ThemeToggle />
          </div>
          <div className="shell__content">{children}</div>
        </main>
      </div>
    </div>
  )
}
