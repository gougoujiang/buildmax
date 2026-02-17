import { useState } from "react"
import type { Artifact, Project } from "../lib/types"
import { navigate } from "../lib/router"
import { createProject, createTask } from "../lib/api"
import { PromptArea } from "../components/PromptArea"
import { CreateProjectModal } from "../components/CreateProjectModal"

interface ProjectsProps {
  workspaceId: string
  projects: Project[]
  artifacts: Artifact[]
  token?: string
  onRefetchProjects?: () => void
  onRefetchWorkspaceTasks?: () => void
  onRefetchArtifacts?: () => void
  onViewArtifact?: (artifactId: string) => void
}

export function Projects({
  workspaceId,
  projects,
  artifacts,
  token,
  onRefetchProjects,
  onRefetchWorkspaceTasks,
  onRefetchArtifacts: _onRefetchArtifacts,
  onViewArtifact,
}: ProjectsProps) {
  const [prompt, setPrompt] = useState("")
  const [showNewProject, setShowNewProject] = useState(false)
  const [creatingProject, setCreatingProject] = useState(false)
  const [createProjError, setCreateProjError] = useState<string | null>(null)
  const [runningTask, setRunningTask] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)

  async function handleRun() {
    const input = prompt.trim()
    if (!input || !token || runningTask) return
    setRunningTask(true)
    setRunError(null)
    try {
      await createTask(workspaceId, { input }, token)
      setPrompt("")
      onRefetchWorkspaceTasks?.()
      navigate({ name: "activity", workspaceId })
    } catch (err) {
      setRunError(err instanceof Error ? err.message : "Failed to run task")
    } finally {
      setRunningTask(false)
    }
  }

  async function handleCreateProject(name: string, description: string) {
    if (!token || !onRefetchProjects) return
    setCreatingProject(true)
    setCreateProjError(null)
    try {
      const p = await createProject(workspaceId, { name, description: description || undefined }, token)
      onRefetchProjects()
      setShowNewProject(false)
      navigate({ name: "project", workspaceId, projectId: p.id })
    } catch (err) {
      setCreateProjError(err instanceof Error ? err.message : "Failed to create project")
    } finally {
      setCreatingProject(false)
    }
  }

  return (
    <div className="page-workspace">
      <PromptArea
        value={prompt}
        onChange={(v) => { setPrompt(v); setRunError(null) }}
        onRun={handleRun}
      />
      {runError && (
        <p className="page-workspace__error" role="alert">
          {runError}
        </p>
      )}

      {/* Project list */}
      <section className="page-workspace__projects">
        <div className="page-workspace__heading-row">
          <h2 className="page-workspace__heading">Projects</h2>
          {token && onRefetchProjects && (
            <button
              type="button"
              className="page-workspace__new-project"
              onClick={() => { setCreateProjError(null); setShowNewProject(true) }}
            >
              New project
            </button>
          )}
        </div>
        <ul className="page-workspace__list">
          {projects.map((p) => (
            <li key={p.id} className="page-workspace__project-card">
              <button
                type="button"
                className="page-workspace__project-link"
                onClick={() =>
                  navigate({
                    name: "project",
                    workspaceId,
                    projectId: p.id,
                  })
                }
              >
                <span className="page-workspace__project-name">{p.name}</span>
                <span className="page-workspace__project-status">
                  {p.status === "paused" ? "Paused" : "Active"}
                </span>
                <span className="page-workspace__project-time">
                  {p.updatedAtLabel}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </section>

      {/* Artifacts */}
      <section className="page-workspace__artifacts">
        <h2 className="page-workspace__heading">Recent artifacts</h2>
        {artifacts.length === 0 ? (
          <p className="page-workspace__empty">No artifacts yet.</p>
        ) : (
          <ul className="page-workspace__artifact-list">
            {artifacts.map((a) => (
              <li key={a.id} className="page-workspace__artifact-card">
                <div className="page-workspace__artifact-main">
                  <span className="page-workspace__artifact-title">{a.title}</span>
                  <span className="page-workspace__artifact-time">{a.timeLabel}</span>
                </div>
                <div className="page-workspace__artifact-ids">
                  <span title="workspace_id">{a.workspaceId}</span>
                  <span title="task_id">{a.taskId}</span>
                  {a.projectId && <span title="project_id">{a.projectId}</span>}
                  <span title="artifact_id">{a.id}</span>
                </div>
                {onViewArtifact && (
                  <button
                    type="button"
                    className="page-workspace__artifact-view"
                    onClick={() => onViewArtifact(a.id)}
                  >
                    View
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      <CreateProjectModal
        open={showNewProject}
        loading={creatingProject}
        error={createProjError}
        onClose={() => setShowNewProject(false)}
        onCreate={handleCreateProject}
      />
    </div>
  )
}
