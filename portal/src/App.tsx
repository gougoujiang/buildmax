import { useHashRoute } from "./router"
import {
  MOCK_WORKSPACE,
  listProjectsForWorkspace,
  getProjectById,
  listTasksForProject,
  listArtifactsForProject,
  getTaskById,
  getArtifactById,
} from "./mockData"
import { AppShell } from "./components/AppShell"
import { WorkspaceHome } from "./pages/WorkspaceHome"
import { ProjectDashboard } from "./pages/ProjectDashboard"
import { TaskDetail } from "./pages/TaskDetail"
import { ArtifactViewer } from "./pages/ArtifactViewer"

function App() {
  const route = useHashRoute()
  const projects = listProjectsForWorkspace(MOCK_WORKSPACE.id)

  // Resolve the selected project id from any route that carries one
  const selectedProjectId =
    route.name !== "workspace" ? route.projectId : null

  // Render the page for the current route
  function renderPage() {
    switch (route.name) {
      case "workspace":
        return <WorkspaceHome projects={projects} />

      case "project": {
        const project = getProjectById(route.projectId)
        if (!project) return <WorkspaceHome projects={projects} />
        return (
          <ProjectDashboard
            project={project}
            tasks={listTasksForProject(project.id)}
            artifacts={listArtifactsForProject(project.id)}
          />
        )
      }

      case "task": {
        const task = getTaskById(route.projectId, route.taskId)
        if (!task) {
          // Fallback to project dashboard if task not found
          const project = getProjectById(route.projectId)
          if (!project) return <WorkspaceHome projects={projects} />
          return (
            <ProjectDashboard
              project={project}
              tasks={listTasksForProject(project.id)}
              artifacts={listArtifactsForProject(project.id)}
            />
          )
        }
        return <TaskDetail task={task} />
      }

      case "artifact": {
        const artifact = getArtifactById(route.projectId, route.artifactId)
        if (!artifact) {
          const project = getProjectById(route.projectId)
          if (!project) return <WorkspaceHome projects={projects} />
          return (
            <ProjectDashboard
              project={project}
              tasks={listTasksForProject(project.id)}
              artifacts={listArtifactsForProject(project.id)}
            />
          )
        }
        return <ArtifactViewer artifact={artifact} />
      }
    }
  }

  return (
    <AppShell
      workspaceName={MOCK_WORKSPACE.name}
      projects={projects}
      selectedProjectId={selectedProjectId}
      route={route}
    >
      {renderPage()}
    </AppShell>
  )
}

export default App
