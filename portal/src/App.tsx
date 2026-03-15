import { useEffect, useState } from "react"
import { navigate } from "./router"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "@buildmax/gui"
import { WorkspaceProvider, useWorkspace } from "./contexts/WorkspaceContext"
import { Layout } from "./layout/Layout"
import { ArtifactContentModal } from "./components/ArtifactContentModal"
import { WorkspaceRouter } from "./components/WorkspaceRouter"
import { useWorkspaceConversations } from "./hooks/useWorkspaceConversations"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"

function AppContent() {
  const { token, user, logout } = useAuth()
  const {
    route,
    profiles,
    loadingProfiles,
  } = useWorkspace()
  const {
    data: profileConversations,
    refetch: refetchProfileConversations,
  } = useWorkspaceConversations(route.profileId, token)

  const [viewArtifact, setViewArtifact] = useState<{ chatRunId: string } | null>(null)

  const defaultProfileId = profiles[0]?.id ?? ""
  const currentProfileFromRoute = profiles.find((profile) => profile.id === route.profileId)
  const needsRedirect = !route.profileId || !currentProfileFromRoute

  useEffect(() => {
    if (needsRedirect && defaultProfileId) {
      navigate({ name: "home", profileId: defaultProfileId })
    }
  }, [needsRedirect, defaultProfileId])

  if (!token) {
    const authHash = window.location.hash.replace(/^#\/?/, "").toLowerCase()
    if (authHash === "signup") return <SignUp />
    return <Login />
  }

  if (loadingProfiles) {
    return null
  }

  if (needsRedirect) {
    return null
  }

  const currentProfile = { id: currentProfileFromRoute!.id, name: currentProfileFromRoute!.name }

  return (
    <>
      <Layout
        route={route}
        currentProfile={currentProfile}
        profiles={profiles}
        profileConversations={profileConversations}
        user={user!}
        onLogout={logout}
      >
        <WorkspaceRouter
          workspaceConversations={profileConversations}
          onRefetchWorkspaceConversations={refetchProfileConversations}
          onViewArtifact={setViewArtifact}
        />
      </Layout>
      {viewArtifact && token && (
        <ArtifactContentModal
          open={true}
          chatRunId={viewArtifact.chatRunId}
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
        <WorkspaceProvider>
          <AppContent />
        </WorkspaceProvider>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
