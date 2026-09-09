import { useEffect } from "react"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "@buildmax/gui"
import { AppProvider, useApp } from "./contexts/AppContext"
import { WebSocketProvider } from "./contexts/WebSocketContext"
import { SpaceProvider, useSpace } from "./contexts/SpaceContext"
import { Layout } from "./layout/Layout"
import { AppRouter } from "./components/AppRouter"
import { useConversations } from "./hooks/useConversations"
import { Login } from "./pages/auth/Login"
import { navigate } from "./router"

function AppContent() {
  const { token, user, logout } = useAuth()
  const { route } = useApp()
  const { currentSpaceId, loading: spacesLoading, setCurrentSpaceId } = useSpace()
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(token, currentSpaceId)

  useEffect(() => {
    if (!token || !currentSpaceId) return
    if (route.name !== "login") return
    navigate({ name: "chat", spaceId: currentSpaceId })
  }, [token, route, currentSpaceId])

  // The URL is authoritative for Space context (docs/design/portal-navigation
  // -and-space-context.md): reconcile the shell to whatever Space a direct
  // link or a reload just named, so the sidebar and switcher agree with what
  // is on screen. This only ever sets context, never navigates -- the one
  // place a Space change is a genuine user action, and so the one place that
  // also redirects the route, is the switcher itself (Sidebar.tsx).
  useEffect(() => {
    if (!("spaceId" in route) || !route.spaceId || route.spaceId === currentSpaceId) return
    setCurrentSpaceId(route.spaceId)
  }, [route, currentSpaceId, setCurrentSpaceId])

  if (!token) {
    return <Login />
  }

  if (!currentSpaceId) {
    // Every Space-scoped route needs a real Space id before it can render --
    // wait for the account's Spaces to resolve (usually instant: the last
    // selected Space is read back from local storage before this ever renders).
    return <div className="app-loading">{spacesLoading ? "Loading…" : "No space available."}</div>
  }

  return (
    <Layout
      route={route}
      conversations={conversations}
      user={user!}
      onLogout={logout}
    >
      <AppRouter
        conversations={conversations}
        onRefetchConversations={refetchConversations}
        userId={user!.id}
      />
    </Layout>
  )
}

function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <SpaceProvider>
          <WebSocketProvider>
            <AppProvider>
              <AppContent />
            </AppProvider>
          </WebSocketProvider>
        </SpaceProvider>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
