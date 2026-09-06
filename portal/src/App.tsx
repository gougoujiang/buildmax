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
  const { currentSpaceId } = useSpace()
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(token, currentSpaceId)
  const previousSpaceIdRef = useRef<string | null>(currentSpaceId)

  useEffect(() => {
    if (!token) return
    if (route.name !== "login") return
    navigate({ name: "home" })
  }, [token, route])

  useEffect(() => {
    const previousSpaceId = previousSpaceIdRef.current
    previousSpaceIdRef.current = currentSpaceId
    if (!previousSpaceId || previousSpaceId === currentSpaceId) return

    setPendingConversation(null)
    if (route.name === "conversation") {
      navigate({ name: "home" })
      return
    }
    if (route.name === "issue") {
      navigate({ name: "issues" })
      return
    }
    if (route.name === "workflow") {
      navigate({ name: "workflows" })
      return
    }
    if (route.name === "workflowRun") {
      navigate({ name: "workflows" })
      return
    }
    // An artifact belongs to one space, so the detail open before the switch is
    // not readable after it -- leaving it would render the 404 page.
    if (route.name === "artifact") {
      navigate({ name: "artifacts" })
    }
  }, [currentSpaceId, route, setPendingConversation])

  if (!token) {
    return <Login />
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
