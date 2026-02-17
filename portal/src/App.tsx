import { useEffect, useState } from "react"
import type { Project, Task } from "./lib"
import { useHashRoute, navigate } from "./lib"
import { createWorkspace } from "./lib/api"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { useWorkspaceData } from "./hooks/useWorkspaceData"
import { AppShell } from "./components/AppShell"
import { CreateWorkspaceModal } from "./components/CreateWorkspaceModal"
import { ArtifactContentModal } from "./components/ArtifactContentModal"
import { Login } from "./pages/Login"
import { SignUp } from "./pages/SignUp"
import { Projects } from "./pages/Projects"
import { Project as ProjectView } from "./pages/Project"
import { TaskDetail } from "./pages/TaskDetail"
import { Activity } from "./pages/Activity"
import { Explore } from "./pages/Explore"

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

  const projectIdFromRoute = "projectId" in route ? route.projectId : undefined
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

  function getProjectById(projectId: string): Project | undefined {
    return projects.find((p) => p.id === projectId)
  }

  function getProjectName(projectId: string): string {
    return projects.find((p) => p.id === projectId)?.name ?? "Project"
  }

  function getTaskForDetail(projectId: string | undefined, taskId: string): Task | undefined {
    const fromProject = projectId ? tasks.find((t) => t.id === taskId) : undefined
    if (fromProject) return fromProject
    return workspaceTasks.find((t) => t.id === taskId)
  }

  function onWorkspaceChange(workspaceId: string) {
    navigate({ name: "workspace", workspaceId })
  }

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

  function renderPage() {
    const fallbackHome = (
      <Projects
        workspaceId={route.workspaceId}
        projects={projects}
        artifacts={artifacts}
        token={token ?? undefined}
        onRefetchProjects={refetchProjects}
        onRefetchWorkspaceTasks={refetchWorkspaceTasks}
        onRefetchArtifacts={refetchArtifacts}
        onViewArtifact={(artifactId: string) => setViewArtifact({ workspaceId: route.workspaceId, artifactId })}
      />
    )
    const fallbackProject = (project: Project) => (
      <ProjectView
        workspaceId={route.workspaceId}
        project={project}
        tasks={projectIdFromRoute === project.id ? tasks : []}
        artifacts={projectIdFromRoute === project.id ? artifacts : []}
        token={token ?? undefined}
        onRefetchTasks={refetchTasks}
        onRefetchArtifacts={refetchArtifacts}
        onViewArtifact={(artifactId: string) => setViewArtifact({ workspaceId: route.workspaceId, artifactId })}
      />
    )

    if (route.name === "workspace") return fallbackHome
    if (route.name === "activity") {
      return (
        <Activity
          workspaceId={route.workspaceId}
          tasks={workspaceTasks}
          artifacts={artifacts}
          getProjectName={getProjectName}
          onViewArtifact={(artifactId: string) => setViewArtifact({ workspaceId: route.workspaceId, artifactId })}
        />
      )
    }
    if (route.name === "explore") return <Explore workspaceId={route.workspaceId} />

    // project and task: resolve entity or fallback to home/project
    const project = "projectId" in route ? getProjectById(route.projectId) : undefined
    const projectMismatch = !project || project.workspaceId !== route.workspaceId
    if (route.name === "project") {
      if (projectMismatch) return fallbackHome
      return fallbackProject(project)
    }
    if (route.name === "task") {
      const task = getTaskForDetail(route.projectId, route.taskId)
      if (!task) {
        if (projectMismatch) return fallbackHome
        return fallbackProject(project!)
      }
      return <TaskDetail task={task} />
    }
    return fallbackHome
  }

  return (
    <>
      <AppShell
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
        projects={projects}
        route={route}
        onWorkspaceChange={onWorkspaceChange}
        onNewWorkspace={handleNewWorkspace}
        user={user!}
        onLogout={logout}
      >
        {renderPage()}
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
