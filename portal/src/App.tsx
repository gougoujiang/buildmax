import { useEffect } from "react"
import { useHashRoute, navigate } from "./router"
import {
  listWorkspaces,
  getWorkspaceById,
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
  const selectedProjectId =
    route.name !== "workspace" ? route.projectId : null

  function onWorkspaceChange(workspaceId: string) {
    navigate({ name: "workspace", workspaceId })
  }

  function renderPage() {
    switch (route.name) {
      case "workspace":
        return (
          <WorkspaceHome
            workspaceId={route.workspaceId}
            projects={projects}
          />
        )

      case "project": {
        const project = getProjectById(route.projectId)
        if (!project || project.workspaceId !== route.workspaceId) {
          return (
            <WorkspaceHome
              workspaceId={route.workspaceId}
              projects={projects}
            />
          )
        }
        return (
          <ProjectDashboard
            workspaceId={route.workspaceId}
            project={project}
            tasks={listTasksForProject(project.id)}
            artifacts={listArtifactsForProject(project.id)}
          />
        )
      }

      case "task": {
        const task = getTaskById(route.projectId, route.taskId)
        if (!task) {
          const project = getProjectById(route.projectId)
          if (!project || project.workspaceId !== route.workspaceId) {
            return (
              <WorkspaceHome
                workspaceId={route.workspaceId}
                projects={projects}
              />
            )
          }
          return (
            <ProjectDashboard
              workspaceId={route.workspaceId}
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
          if (!project || project.workspaceId !== route.workspaceId) {
            return (
              <WorkspaceHome
                workspaceId={route.workspaceId}
                projects={projects}
              />
            )
          }
          return (
            <ProjectDashboard
              workspaceId={route.workspaceId}
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
      currentWorkspace={currentWorkspace}
      workspaces={workspaces}
      projects={projects}
      selectedProjectId={selectedProjectId}
      route={route}
      onWorkspaceChange={onWorkspaceChange}
    >
      {renderPage()}
    </AppShell>
  )
}

export default App
