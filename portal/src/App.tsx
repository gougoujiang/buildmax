import { useEffect } from "react"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "@buildmax/gui"
import { AppProvider, useApp } from "./contexts/AppContext"
import { WebSocketProvider } from "./contexts/WebSocketContext"
import { Layout } from "./layout/Layout"
import { AppRouter } from "./components/AppRouter"
import { useConversations } from "./hooks/useConversations"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"
import { navigate } from "./router"

function AppContent() {
  const { token, user, logout } = useAuth()
  const { route } = useApp()
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(token)

  useEffect(() => {
    if (!token) return
    if (route.name !== "login" && route.name !== "signup") return
    navigate({ name: "home" })
  }, [token, route])

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
      />
    </Layout>
  )
}

function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <WebSocketProvider>
          <AppProvider>
            <AppContent />
          </AppProvider>
        </WebSocketProvider>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
