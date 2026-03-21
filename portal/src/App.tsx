import { useEffect, useState } from "react"
import { navigate } from "./router"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "@buildmax/gui"
import { AppProvider, useApp } from "./contexts/AppContext"
import { WebSocketProvider } from "./contexts/WebSocketContext"
import { Layout } from "./layout/Layout"
import { ArtifactContentModal } from "./components/ArtifactContentModal"
import { AppRouter } from "./components/AppRouter"
import { useConversations } from "./hooks/useConversations"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"

function AppContent() {
  const { token, user, logout } = useAuth()
  const { route } = useApp()
  const userId = user?.id ?? ""
  const {
    data: conversations,
    refetch: refetchConversations,
  } = useConversations(route.profileId, token)

  const [viewArtifact, setViewArtifact] = useState<{ taskRunId: string } | null>(null)

  const needsRedirect = !!userId && (!route.profileId || route.profileId !== userId)

  useEffect(() => {
    if (needsRedirect) {
      navigate({ name: "home", profileId: userId })
    }
  }, [needsRedirect, userId])

  if (!token) {
    const authHash = window.location.hash.replace(/^#\/?/, "").toLowerCase()
    if (authHash === "signup") return <SignUp />
    return <Login />
  }

  if (needsRedirect) {
    return null
  }

  return (
    <>
      <Layout
        route={route}
        conversations={conversations}
        user={user!}
        onLogout={logout}
      >
        <AppRouter
          conversations={conversations}
          onRefetchConversations={refetchConversations}
          onViewArtifact={setViewArtifact}
        />
      </Layout>
      {viewArtifact && token && (
        <ArtifactContentModal
          open={true}
          taskRunId={viewArtifact.taskRunId}
          token={token}
          onClose={() => setViewArtifact(null)}
        />
      )}
    </>
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
