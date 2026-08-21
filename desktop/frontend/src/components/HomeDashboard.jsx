import { folderBaseName, formatSessionMeta } from '../lib/format';

export function HomeDashboard({ recentSessions, recentProjects, projectByWorkspace, onSelectSession, onOpenProject, onCreateProject }) {
  return (
    <div className="page-home">
      <div className="page-home__header">
        <div>
          <h1 className="page-home__title">Continue your work</h1>
          <p className="page-home__subtitle">Pick up a recent chat or open a project workspace.</p>
        </div>
        <button type="button" className="page-home__primary" onClick={onCreateProject}>
          New Project
        </button>
      </div>

      <div className="page-home__grid">
        <section className="page-home__section" aria-label="Recent chats">
          <div className="page-home__section-head">
            <h2>Recent chats</h2>
          </div>
          {recentSessions.length === 0 ? (
            <p className="page-home__empty">No recent chats yet.</p>
          ) : (
            <div className="page-home__list">
              {recentSessions.map((s) => {
                const project = projectByWorkspace.get(s.workspace);
                return (
                  <button
                    key={s.id}
                    type="button"
                    className="page-home__item"
                    onClick={() => onSelectSession(s.id)}
                  >
                    <span className="page-home__item-title">{s.pinned ? '★ ' : ''}{s.title?.trim() || 'Chat'}</span>
                    <span className="page-home__item-meta">
                      {project?.name || folderBaseName(s.workspace) || 'Unknown project'} · {formatSessionMeta(s.created_at)}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </section>

        <section className="page-home__section" aria-label="Recent projects">
          <div className="page-home__section-head">
            <h2>Recent projects</h2>
          </div>
          {recentProjects.length === 0 ? (
            <p className="page-home__empty">Create a project once, then it will stay here for quick access.</p>
          ) : (
            <div className="page-home__list">
              {recentProjects.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className="page-home__item"
                  onClick={() => onOpenProject(p)}
                >
                  <span className="page-home__item-title">{p.name}</span>
                  <span className="page-home__item-meta">{p.folder_path}</span>
                </button>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

// --- ApprovalPanel ---
