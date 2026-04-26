import { useEffect, useRef } from "react"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "@buildmax/gui"
import { AppProvider, useApp } from "./contexts/AppContext"
import { WebSocketProvider } from "./contexts/WebSocketContext"
import { TeamProvider, useTeam } from "./contexts/TeamContext"
import { Layout } from "./layout/Layout"
import { AppRouter } from "./components/AppRouter"
import { useConversations } from "./hooks/useConversations"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"
import { navigate } from "./router"

function AppContent() {
  const { token, user, logout } = useAuth()
  const { route, setPendingConversation } = useApp()
  const { currentTeamId } = useTeam()
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(token, currentTeamId)
  const previousTeamIdRef = useRef<string | null>(currentTeamId)

  useEffect(() => {
    if (!token) return
    if (route.name !== "login" && route.name !== "signup") return
    navigate({ name: "home" })
  }, [token, route])

  useEffect(() => {
    const previousTeamId = previousTeamIdRef.current
    previousTeamIdRef.current = currentTeamId
    if (!previousTeamId || previousTeamId === currentTeamId) return

    setPendingConversation(null)
    if (route.name === "conversation") {
      navigate({ name: "home" })
      return
    }
    if (route.name === "issue") {
      navigate({ name: "issues" })
    }
  }, [currentTeamId, route, setPendingConversation])

  if (!token) {
    if (route.name === "signup") return <SignUp />
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
        <TeamProvider>
          <WebSocketProvider>
            <AppProvider>
              <AppContent />
            </AppProvider>
          </WebSocketProvider>
        </TeamProvider>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
