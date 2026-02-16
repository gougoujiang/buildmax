import { useState } from "react"
import type { Project } from "../lib/types"
import { navigate } from "../lib/router"
import { createProject } from "../lib/api"
import { PromptArea } from "../components/PromptArea"
import { CreateProjectModal } from "../components/CreateProjectModal"

interface ProjectsProps {
  workspaceId: string
  projects: Project[]
  token?: string
  onRefetchProjects?: () => void
}

const QUICK_ACTIONS = [
  "Create monthly report",
  "Summarize meeting",
  "Analyze CSV",
  "Draft email",
]

export function Projects({
  workspaceId,
  projects,
  token,
  onRefetchProjects,
}: ProjectsProps) {
  const [prompt, setPrompt] = useState("")
  const [showNewProject, setShowNewProject] = useState(false)
  const [creatingProject, setCreatingProject] = useState(false)
  const [createProjError, setCreateProjError] = useState<string | null>(null)

  function handleRun() {
    if (prompt.trim()) {
      console.log("Run (no-op):", prompt.trim())
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
        onChange={setPrompt}
        onRun={handleRun}
      />

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

      {/* Quick actions */}
      <section className="page-workspace__actions">
        <h2 className="page-workspace__heading">Quick Actions</h2>
        <div className="page-workspace__action-grid">
          {QUICK_ACTIONS.map((label) => (
            <button
              key={label}
              type="button"
              className="page-workspace__action-btn"
              onClick={() => {
                /* no-op */
              }}
            >
              {label}
            </button>
          ))}
        </div>
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
