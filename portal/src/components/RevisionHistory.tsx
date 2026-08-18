interface RevisionEntry {
  id: string
  revision: number
  createdBy: string
  createdLabel: string
  summary?: string | null
}

interface RevisionHistoryProps {
  title: string
  entries: RevisionEntry[]
  currentRevision: number
  loading: boolean
  error: string | null
  canRestore: boolean
  restoringRevision: number | null
  onRestore: (revision: number) => void
}

/**
 * Shared history list for agents and workflows. Restoring writes an older
 * version's content back as a new version, so the button says Restore rather
 * than anything that suggests the versions since are discarded.
 */
export function RevisionHistory({
  title,
  entries,
  currentRevision,
  loading,
  error,
  canRestore,
  restoringRevision,
  onRestore,
}: RevisionHistoryProps) {
  return (
    <section className="revision-history">
      <div className="revision-history__head">
        <h3 className="revision-history__title">{title}</h3>
        {currentRevision > 0 ? (
          <span className="page-activity__meta">Current: v{currentRevision}</span>
        ) : null}
      </div>
      {error ? <p className="modal__error">{error}</p> : null}
      {loading ? (
        <p className="page-activity__empty">Loading history…</p>
      ) : entries.length === 0 ? (
        <p className="page-activity__empty">No history recorded yet.</p>
      ) : (
        <ol className="revision-history__list">
          {entries.map((entry) => (
            <li key={entry.id} className="revision-history__item">
              <div className="revision-history__item-head">
                <strong>v{entry.revision}</strong>
                <span className="page-activity__meta">
                  {entry.createdBy} · {entry.createdLabel}
                </span>
                {canRestore && entry.revision !== currentRevision ? (
                  <button
                    type="button"
                    className="page-activity__action-btn"
                    disabled={restoringRevision !== null}
                    onClick={() => onRestore(entry.revision)}
                  >
                    {restoringRevision === entry.revision ? "Restoring…" : "Restore"}
                  </button>
                ) : null}
              </div>
              {entry.summary ? (
                <pre className="revision-history__summary">{entry.summary}</pre>
              ) : null}
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
