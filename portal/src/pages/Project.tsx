import { useState } from "react"
import type { Artifact, Project, Task, ViewArtifactParams } from "../lib/types"
import { taskStatusIcon } from "../lib/taskStatus"
import { navigate } from "../lib/router"
import { createTask } from "../lib/api"
import { PromptArea } from "../components/PromptArea"

interface ProjectProps {
  workspaceId: string
  project: Project
  tasks: Task[]
  artifacts: Artifact[]
  token?: string
  onRefetchTasks?: () => void
  onRefetchArtifacts?: () => void
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function Project({
  workspaceId,
  project,
  tasks,
  artifacts,
  token,
  onRefetchTasks,
  onRefetchArtifacts,
  onViewArtifact,
}: ProjectProps) {
  const [prompt, setPrompt] = useState("")
  const [, setCreating] = useState(false)

  async function handleRun() {
    const input = prompt.trim()
    if (!input) return
    if (token && onRefetchTasks) {
      setCreating(true)
      try {
        await createTask(workspaceId, { input, project_id: project.id }, token)
        setPrompt("")
        onRefetchTasks()
        onRefetchArtifacts?.()
      } catch {
        // Error could be shown in UI; for now just stop loading
      } finally {
        setCreating(false)
      }
    } else {
      console.log("Run (no-op):", input)
    }
  }

  return (
    <div className="page-project">
      {/* Project overview */}
      <section className="page-project__overview">
        <h1 className="page-project__title">{project.name}</h1>
        <p className="page-project__meta">
          Status: <strong>{project.status}</strong> &middot;{" "}
          {project.updatedAtLabel}
        </p>
      </section>

      <section className="page-project__prompt">
        <PromptArea
          value={prompt}
          onChange={setPrompt}
          onRun={handleRun}
          heading="Ask in this project"
          placeholder="Prepare February sales analysis"
          ariaLabel="Ask in this project"
        />
      </section>

      {/* Activity (tasks) — newest first */}
      {tasks.length > 0 && (
        <section className="page-project__activity">
          <h2 className="page-project__section-heading">Activity</h2>
          <ul className="page-project__task-list">
            {[...tasks].reverse().map((t) => (
              <li key={t.id} className="page-project__task-card">
                <button
                  type="button"
                  className="page-project__task-link"
                  onClick={() =>
                    navigate({
                      name: "task",
                      workspaceId,
                      projectId: project.id,
                      taskId: t.id,
                    })
                  }
                >
                  <span className="page-project__task-icon">
                    {taskStatusIcon(t.status)}
                  </span>
                  <span className="page-project__task-title">{t.title}</span>
                  <span className="page-project__task-time">{t.timeLabel}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Artifacts for this project */}
      <section className="page-project__artifacts">
        <h2 className="page-project__section-heading">Artifacts</h2>
        {artifacts.length === 0 ? (
          <p className="page-project__empty">No artifacts in this project yet.</p>
        ) : (
          <ul className="page-project__artifact-list">
            {artifacts.map((a) => (
              <li key={a.id} className="page-project__artifact-card">
                <span className="page-project__artifact-title">{a.title}</span>
                <span className="page-project__artifact-meta">
                  {a.timeLabel} · task: {a.taskId} · artifact: {a.id}
                </span>
                {onViewArtifact && (
                  <button
                    type="button"
                    className="page-project__artifact-view"
                    onClick={() => onViewArtifact({ workspaceId, artifactId: a.id })}
                  >
                    View
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
