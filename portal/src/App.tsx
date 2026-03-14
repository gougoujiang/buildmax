import { useEffect, useState } from "react"
import { navigate } from "./router"
import { getErrorMessage } from "./lib/errorMessage"
import { createWorkspace } from "./lib/api"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { ThemeProvider } from "./contexts/ThemeContext"
import { WorkspaceProvider, useWorkspace } from "./contexts/WorkspaceContext"
import { Layout } from "./layout/Layout"
import { CreateWorkspaceModal } from "./components/CreateWorkspaceModal"
import { ArtifactContentModal } from "./components/ArtifactContentModal"
import { WorkspaceRouter } from "./components/WorkspaceRouter"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"

function AppContent() {
  const { token, user, logout } = useAuth()
  const {
    route,
    workspaces,
    workspaceConversations,
    loadingWorkspaces,
    refetchWorkspaces,
  } = useWorkspace()

  const [showNewWorkspace, setShowNewWorkspace] = useState(false)
  const [viewArtifact, setViewArtifact] = useState<{ workspaceId: string; chatRunId: string } | null>(null)
  const [creatingWorkspace, setCreatingWorkspace] = useState(false)
  const [createWsError, setCreateWsError] = useState<string | null>(null)

  const defaultWorkspaceId = workspaces[0]?.id ?? ""
  const currentWorkspaceFromRoute = workspaces.find((w) => w.id === route.workspaceId)
  const needsRedirect = !route.workspaceId || !currentWorkspaceFromRoute

  useEffect(() => {
    if (needsRedirect && defaultWorkspaceId) {
      navigate({ name: "workspace", workspaceId: defaultWorkspaceId })
    }
  }, [needsRedirect, defaultWorkspaceId])

  if (!token) {
    const authHash = window.location.hash.replace(/^#\/?/, "").toLowerCase()
    if (authHash === "signup") return <SignUp />
    return <Login />
  }

  if (loadingWorkspaces) {
    return null
  }

  if (needsRedirect) {
    return null
  }

  const currentWorkspace = { id: currentWorkspaceFromRoute!.id, name: currentWorkspaceFromRoute!.name }

  function handleNewWorkspace() {
    setCreateWsError(null)
    setShowNewWorkspace(true)
  }

  async function handleCreateWorkspace(name: string) {
    if (!token) return
    setCreatingWorkspace(true)
    setCreateWsError(null)
    try {
      const ws = await createWorkspace({ name }, token)
      await refetchWorkspaces()
      setShowNewWorkspace(false)
      navigate({ name: "workspace", workspaceId: ws.id })
    } catch (err) {
      setCreateWsError(getErrorMessage(err, "Failed to create workspace"))
    } finally {
      setCreatingWorkspace(false)
    }
  }

  return (
    <>
      <Layout
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        onNewWorkspace={handleNewWorkspace}
        workspaceConversations={workspaceConversations}
        user={user!}
        onLogout={logout}
      >
        <WorkspaceRouter onViewArtifact={setViewArtifact} />
      </Layout>
      <CreateWorkspaceModal
        open={showNewWorkspace}
        loading={creatingWorkspace}
        error={createWsError}
        onClose={() => setShowNewWorkspace(false)}
        onCreate={handleCreateWorkspace}
      />
      {viewArtifact && token && (
        <ArtifactContentModal
          open={true}
          workspaceId={viewArtifact.workspaceId}
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