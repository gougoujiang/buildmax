import { useEffect } from "react"
import type { Project } from "./lib"
import { useHashRoute, navigate } from "./lib"
import {
  listWorkspaces,
  getWorkspaceById,
  createWorkspace,
  listProjectsForWorkspace,
  getProjectById,
  listTasksForProject,
  listArtifactsForProject,
  getTaskById,
  getArtifactById,
} from "./data"
import { AppShell } from "./components/AppShell"
import { WorkspaceHome } from "./pages/WorkspaceHome"
import { ProjectDashboard } from "./pages/ProjectDashboard"
import { TaskDetail } from "./pages/TaskDetail"
import { ArtifactViewer } from "./pages/ArtifactViewer"
import { ActivityPage } from "./pages/ActivityPage"
import { ExplorePage } from "./pages/ExplorePage"

function App() {
  const route = useHashRoute()
  const workspaces = listWorkspaces()
  const defaultWorkspaceId = workspaces[0]?.id ?? ""

  // Validate workspace: redirect to first workspace home if missing or invalid
  const currentWorkspaceFromRoute = getWorkspaceById(route.workspaceId)
  const needsRedirect =
    !route.workspaceId || !currentWorkspaceFromRoute

  useEffect(() => {
    if (needsRedirect && defaultWorkspaceId) {
      navigate({ name: "workspace", workspaceId: defaultWorkspaceId })
    }
  }, [needsRedirect, defaultWorkspaceId])

  if (needsRedirect) {
    return null
  }

  const currentWorkspace = currentWorkspaceFromRoute!
  const projects = listProjectsForWorkspace(route.workspaceId)

  function onWorkspaceChange(workspaceId: string) {
    navigate({ name: "workspace", workspaceId })
  }

  function onNewWorkspace() {
    const workspace = createWorkspace("New Workspace")
    navigate({ name: "workspace", workspaceId: workspace.id })
  }

  function renderPage() {
    const fallbackHome = (
      <WorkspaceHome workspaceId={route.workspaceId} projects={projects} />
    )
    const fallbackProject = (project: Project) => (
      <ProjectDashboard
        workspaceId={route.workspaceId}
        project={project}
        tasks={listTasksForProject(project.id)}
        artifacts={listArtifactsForProject(project.id)}
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
        const task = getTaskById(route.projectId, route.taskId)
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
      onNewWorkspace={onNewWorkspace}
    >
      {renderPage()}
    </AppShell>
  )
}

export default App
