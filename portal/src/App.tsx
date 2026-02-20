import { useEffect, useState } from "react"
import { useHashRoute, navigate } from "./lib"
import { createWorkspace } from "./lib/api"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { useWorkspaceData } from "./hooks/useWorkspaceData"
import { AppShell } from "./components/AppShell"
import { CreateWorkspaceModal } from "./components/CreateWorkspaceModal"
import { ArtifactContentModal } from "./components/ArtifactContentModal"
import { WorkspaceRouter } from "./components/WorkspaceRouter"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"

function AppContent() {
  const { token, user, logout } = useAuth()
  const route = useHashRoute()
  const {
    workspaces,
    projects,
    tasks,
    workspaceTasks,
    artifacts,
    loadingWorkspaces,
    refetchWorkspaces,
    refetchProjects,
    refetchTasks,
    refetchWorkspaceTasks,
    refetchArtifacts,
  } = useWorkspaceData(token, route)

  const [showNewWorkspace, setShowNewWorkspace] = useState(false)
  const [viewArtifact, setViewArtifact] = useState<{ workspaceId: string; artifactId: string } | null>(null)
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
      refetchWorkspaces()
      setShowNewWorkspace(false)
      navigate({ name: "workspace", workspaceId: ws.id })
    } catch (err) {
      setCreateWsError(err instanceof Error ? err.message : "Failed to create workspace")
    } finally {
      setCreatingWorkspace(false)
    }
  }

  return (
    <>
      <AppShell
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        projects={projects}
        route={route}
        onWorkspaceChange={(workspaceId) => navigate({ name: "workspace", workspaceId })}
        onNewWorkspace={handleNewWorkspace}
        user={user!}
        onLogout={logout}
      >
        <WorkspaceRouter
          route={route}
          projects={projects}
          tasks={tasks}
          workspaceTasks={workspaceTasks}
          artifacts={artifacts}
          token={token ?? undefined}
          onViewArtifact={setViewArtifact}
          onRefetchProjects={refetchProjects}
          onRefetchTasks={refetchTasks}
          onRefetchWorkspaceTasks={refetchWorkspaceTasks}
          onRefetchArtifacts={refetchArtifacts}
        />
      </AppShell>
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
          artifactId={viewArtifact.artifactId}
          token={token}
          onClose={() => setViewArtifact(null)}
        />
      )}
    </>
  )
}

function App() {
  return (
    <AuthProvider>
      <AppContent />
    </AuthProvider>
  )
}

export default App
