import { useCallback, useEffect, useState } from "react"
import type { Project, Task } from "./lib"
import { useHashRoute, navigate } from "./lib"
import { listArtifactsForProject, getTaskById, getArtifactById } from "./data"
import {
  getWorkspaces,
  createWorkspace,
  getProjects,
  getTasks,
  apiProjectToProject,
  apiTaskToTask,
  type ApiWorkspace,
} from "./lib/api"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { AppShell } from "./components/AppShell"
import { CreateWorkspaceModal } from "./components/CreateWorkspaceModal"
import { Login } from "./pages/Login"
import { Projects } from "./pages/Projects"
import { Project as ProjectView } from "./pages/Project"
import { TaskDetail } from "./pages/TaskDetail"
import { ArtifactViewer } from "./pages/ArtifactViewer"
import { Activity } from "./pages/Activity"
import { Explore } from "./pages/Explore"

function AppContent() {
  const { token, user, logout } = useAuth()
  const route = useHashRoute()
  const [workspaces, setWorkspaces] = useState<ApiWorkspace[]>([])
  const [loadingWorkspaces, setLoadingWorkspaces] = useState(true)
  const [projects, setProjects] = useState<Project[]>([])
  const [, setLoadingProjects] = useState(false)
  const [tasks, setTasks] = useState<Task[]>([])
  const [, setLoadingTasks] = useState(false)
  const [showNewWorkspace, setShowNewWorkspace] = useState(false)
  const [creatingWorkspace, setCreatingWorkspace] = useState(false)
  const [createWsError, setCreateWsError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) {
      setWorkspaces([])
      setLoadingWorkspaces(false)
      return
    }
    setLoadingWorkspaces(true)
    getWorkspaces(token)
      .then(setWorkspaces)
      .catch(() => setWorkspaces([]))
      .finally(() => setLoadingWorkspaces(false))
  }, [token])

  const refetchProjects = useCallback(() => {
    if (!token || !route.workspaceId) return
    setLoadingProjects(true)
    getProjects(route.workspaceId, token)
      .then((list) => setProjects(list.map(apiProjectToProject)))
      .catch(() => setProjects([]))
      .finally(() => setLoadingProjects(false))
  }, [token, route.workspaceId])

  useEffect(() => {
    if (!token || !route.workspaceId) {
      setProjects([])
      return
    }
    setLoadingProjects(true)
    getProjects(route.workspaceId, token)
      .then((list) => setProjects(list.map(apiProjectToProject)))
      .catch(() => setProjects([]))
      .finally(() => setLoadingProjects(false))
  }, [token, route.workspaceId])

  const projectIdFromRoute = "projectId" in route ? route.projectId : undefined
  const refetchTasks = useCallback(() => {
    if (!token || !route.workspaceId || !projectIdFromRoute) return
    setLoadingTasks(true)
    getTasks(route.workspaceId, token, projectIdFromRoute)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
      .finally(() => setLoadingTasks(false))
  }, [token, route.workspaceId, projectIdFromRoute])

  useEffect(() => {
    if (!token || !route.workspaceId || !projectIdFromRoute) {
      setTasks([])
      return
    }
    setLoadingTasks(true)
    getTasks(route.workspaceId, token, projectIdFromRoute)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
      .finally(() => setLoadingTasks(false))
  }, [token, route.workspaceId, projectIdFromRoute])

  const defaultWorkspaceId = workspaces[0]?.id ?? ""
  const currentWorkspaceFromRoute = workspaces.find((w) => w.id === route.workspaceId)
  const needsRedirect = !route.workspaceId || !currentWorkspaceFromRoute

  useEffect(() => {
    if (needsRedirect && defaultWorkspaceId) {
      navigate({ name: "workspace", workspaceId: defaultWorkspaceId })
    }
  }, [needsRedirect, defaultWorkspaceId])

  if (!token) {
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
      const updated = await getWorkspaces(token)
      setWorkspaces(updated)
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
        token={token ?? undefined}
        onRefetchProjects={refetchProjects}
      />
    )
    const fallbackProject = (project: Project) => (
      <ProjectView
        workspaceId={route.workspaceId}
        project={project}
        tasks={projectIdFromRoute === project.id ? tasks : []}
        artifacts={listArtifactsForProject(project.id)}
        token={token ?? undefined}
        onRefetchTasks={refetchTasks}
      />
    )

    switch (route.name) {
      case "workspace":
        return fallbackHome

      case "project": {
        const project = getProjectById(route.projectId)
        if (!project || project.workspaceId !== route.workspaceId) {
          return fallbackHome
        }
        return fallbackProject(project)
      }

      case "task": {
        const taskFromApi =
          route.projectId && tasks.find((t) => t.id === route.taskId)
        const task = taskFromApi ?? getTaskById(route.projectId, route.taskId)
        if (!task) {
          const project = getProjectById(route.projectId)
          if (!project || project.workspaceId !== route.workspaceId) {
            return fallbackHome
          }
          return fallbackProject(project)
        }
        return <TaskDetail task={task} />
      }

      case "artifact": {
        const artifact = getArtifactById(route.projectId, route.artifactId)
        if (!artifact) {
          const project = getProjectById(route.projectId)
          if (!project || project.workspaceId !== route.workspaceId) {
            return fallbackHome
          }
          return fallbackProject(project)
        }
        return <ArtifactViewer artifact={artifact} />
      }

      case "activity":
        return <Activity workspaceId={route.workspaceId} />

      case "explore":
        return <Explore workspaceId={route.workspaceId} />
    }
  }

  return (
    <>
      <AppShell
        currentWorkspace={currentWorkspace}
        workspaces={workspaces}
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
