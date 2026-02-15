import type { Project } from "../types"
import { navigate } from "../router"

interface ProjectsSidebarProps {
  projects: Project[]
  selectedProjectId: string | null
}

export function ProjectsSidebar({
  projects,
  selectedProjectId,
}: ProjectsSidebarProps) {
  return (
    <aside className="sidebar">
      <h2 className="sidebar__heading">Projects</h2>
      <ul className="sidebar__list">
        {projects.map((p) => (
          <li key={p.id} className="sidebar__item">
            <button
              type="button"
              className={
                "sidebar__link" +
                (p.id === selectedProjectId ? " sidebar__link--active" : "")
              }
              onClick={() => navigate({ name: "project", projectId: p.id })}
            >
              <span className="sidebar__project-name">{p.name}</span>
              <span className="sidebar__project-meta">
                {p.status === "paused" ? "Paused" : p.updatedAtLabel}
              </span>
            </button>
          </li>
        ))}
      </ul>
      <button
        type="button"
        className="sidebar__new-project"
        onClick={() => {
          /* no-op placeholder */
        }}
      >
        + New Project
      </button>
    </aside>
  )
}
