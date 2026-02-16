import { useState } from "react"
import type { Project, Task, Artifact } from "../lib/types"
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
}

function statusIcon(status: Task["status"]): string {
  switch (status) {
    case "success":
      return "\u2705"
    case "running":
      return "\u23f3"
    case "failed":
      return "\u274c"
    case "canceled":
      return "\u26d4"
    case "pending":
      return "\ud83d\udd50"
  }
}

export function Project({
  workspaceId,
  project,
  tasks,
  artifacts,
  token,
  onRefetchTasks,
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

      {/* Artifacts */}
      {artifacts.length > 0 && (
        <section className="page-project__artifacts">
          <h2 className="page-project__section-heading">Artifacts</h2>
          <ul className="page-project__artifact-list">
            {artifacts.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  className="page-project__artifact-link"
                  onClick={() =>
                    navigate({
                      name: "artifact",
                      workspaceId,
                      projectId: project.id,
                      artifactId: a.id,
                    })
                  }
                >
                  {a.title}
                  <span className="page-project__artifact-kind">{a.kind}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

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
                    {statusIcon(t.status)}
                  </span>
                  <span className="page-project__task-title">{t.title}</span>
                  <span className="page-project__task-time">{t.timeLabel}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
