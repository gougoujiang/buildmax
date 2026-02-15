import { useState } from "react"
import type { Project } from "../types"
import { navigate } from "../router"

interface WorkspaceHomeProps {
  projects: Project[]
}

const QUICK_ACTIONS = [
  "Create monthly report",
  "Summarize meeting",
  "Analyze CSV",
  "Draft email",
]

export function WorkspaceHome({ projects }: WorkspaceHomeProps) {
  const [prompt, setPrompt] = useState("")

  function handleRun() {
    if (prompt.trim()) {
      console.log("Run (no-op):", prompt.trim())
    }
  }

  return (
    <div className="page-workspace">
      {/* Prompt area */}
      <section className="prompt-area">
        <h2 className="prompt-area__heading">
          What would you like to accomplish?
        </h2>
        <input
          type="text"
          className="prompt-area__input"
          placeholder="Help me prepare this month's sales analysis"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          aria-label="Intent or goal"
        />
        <button type="button" className="prompt-area__button" onClick={handleRun}>
          Run
        </button>
      </section>

      {/* Project list */}
      <section className="page-workspace__projects">
        <h2 className="page-workspace__heading">Projects</h2>
        <ul className="page-workspace__list">
          {projects.map((p) => (
            <li key={p.id} className="page-workspace__project-card">
              <button
                type="button"
                className="page-workspace__project-link"
                onClick={() => navigate({ name: "project", projectId: p.id })}
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
    </div>
  )
}
