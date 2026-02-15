import { useCallback, useEffect, useState } from "react"
import type { Project, Task } from "./lib"
import { useHashRoute, navigate } from "./lib"
import { listArtifactsForProject, getTaskById, getArtifactById } from "./data"
import {
  getWorkspaces,
  getProjects,
  getTasks,
  apiProjectToProject,
  apiTaskToTask,
  type ApiWorkspace,
} from "./lib/api"
import { AuthProvider, useAuth } from "./contexts/AuthContext"
import { AppShell } from "./components/AppShell"
import { LoginPage } from "./pages/LoginPage"
import { WorkspaceHome } from "./pages/WorkspaceHome"
import { ProjectDashboard } from "./pages/ProjectDashboard"
import { TaskDetail } from "./pages/TaskDetail"
import { ArtifactViewer } from "./pages/ArtifactViewer"
import { ActivityPage } from "./pages/ActivityPage"
import { ExplorePage } from "./pages/ExplorePage"

function AppContent() {
  const { token, user, logout } = useAuth()
  const route = useHashRoute()
  const [workspaces, setWorkspaces] = useState<ApiWorkspace[]>([])
  const [loadingWorkspaces, setLoadingWorkspaces] = useState(true)
  const [projects, setProjects] = useState<Project[]>([])
  const [loadingProjects, setLoadingProjects] = useState(false)
  const [tasks, setTasks] = useState<Task[]>([])
  const [loadingTasks, setLoadingTasks] = useState(false)

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

  const refetchTasks = useCallback(() => {
    if (!token || !route.projectId) return
    setLoadingTasks(true)
    getTasks(route.projectId, token)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
      .finally(() => setLoadingTasks(false))
  }, [token, route.projectId])

  useEffect(() => {
    if (!token || !route.projectId) {
      setTasks([])
      return
    }
    setLoadingTasks(true)
    getTasks(route.projectId, token)
      .then((list) => setTasks(list.map(apiTaskToTask)))
      .catch(() => setTasks([]))
      .finally(() => setLoadingTasks(false))
  }, [token, route.projectId])

  const defaultWorkspaceId = workspaces[0]?.id ?? ""
  const currentWorkspaceFromRoute = workspaces.find((w) => w.id === route.workspaceId)
  const needsRedirect = !route.workspaceId || !currentWorkspaceFromRoute

  useEffect(() => {
    if (needsRedirect && defaultWorkspaceId) {
      navigate({ name: "workspace", workspaceId: defaultWorkspaceId })
    }
  }, [needsRedirect, defaultWorkspaceId])

  if (!token) {
    return <LoginPage />
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

  function renderPage() {
    const fallbackHome = (
      <WorkspaceHome
        workspaceId={route.workspaceId}
        projects={projects}
        token={token}
        onRefetchProjects={refetchProjects}
      />
    )
    const fallbackProject = (project: Project) => (
      <ProjectDashboard
        workspaceId={route.workspaceId}
        project={project}
        tasks={route.projectId === project.id ? tasks : []}
        artifacts={listArtifactsForProject(project.id)}
        token={token}
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
        return <ActivityPage workspaceId={route.workspaceId} />

      case "explore":
        return <ExplorePage workspaceId={route.workspaceId} />
    }
  }

  return (
    <AppShell
      currentWorkspace={currentWorkspace}
      workspaces={workspaces}
      route={route}
      onWorkspaceChange={onWorkspaceChange}
      user={user!}
      onLogout={logout}
    >
      {renderPage()}
    </AppShell>
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
