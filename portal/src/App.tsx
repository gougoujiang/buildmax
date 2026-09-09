import { useEffect, useRef } from "react"
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
  const { route, setPendingConversation } = useApp()
  const { currentSpaceId, loading: spacesLoading } = useSpace()
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(token, currentSpaceId)
  const previousSpaceIdRef = useRef<string | null>(currentSpaceId)

  useEffect(() => {
    if (!token || !currentSpaceId) return
    if (route.name !== "login") return
    navigate({ name: "chat", spaceId: currentSpaceId })
  }, [token, route, currentSpaceId])

  useEffect(() => {
    const previousSpaceId = previousSpaceIdRef.current
    previousSpaceIdRef.current = currentSpaceId
    if (!previousSpaceId || !currentSpaceId || previousSpaceId === currentSpaceId) return

    setPendingConversation(null)
    // TODO(slice 3): centralize this into an exhaustive per-route-name table
    // (docs/design/portal-navigation-and-space-context.md) -- today's list
    // still omits `agent` and `task`, so switching Space while viewing either
    // leaves stale data on screen, same as before this slice.
    if (route.name === "chat") {
      navigate({ name: "chat", spaceId: currentSpaceId })
      return
    }
    if (route.name === "issue") {
      navigate({ name: "issues", spaceId: currentSpaceId })
      return
    }
    if (route.name === "workflow") {
      navigate({ name: "workflows", spaceId: currentSpaceId })
      return
    }
    if (route.name === "workflowRun") {
      navigate({ name: "workflows", spaceId: currentSpaceId })
      return
    }
    // An artifact belongs to one space, so the detail open before the switch is
    // not readable after it -- leaving it would render the 404 page.
    if (route.name === "artifact") {
      navigate({ name: "artifacts", spaceId: currentSpaceId })
    }
  }, [currentSpaceId, route, setPendingConversation])

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
