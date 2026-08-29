import { useEffect, useRef, useState } from 'react';

function formatWritten(entry) {
  if (!entry.updated_at) return '';
  const at = Date.parse(entry.updated_at);
  if (Number.isNaN(at)) return '';
  const written = new Date(at).toLocaleString();
  return entry.verified_at ? `${written} · verified ${entry.verified_at}` : written;
}

// MemoryDrawer lists what the project remembers and shows one memory's body.
// It reuses the diff-drawer layout: same shell, list pane, and content pane.
//
// Read-only. A memory is a Markdown file the user can edit directly, and the
// directory is shown so they can; editing here needs the refusal path a
// digest-checked write can take, which is its own piece of work.
export function MemoryDrawer({ projectID, app, onClose }) {
  const [payload, setPayload] = useState(null);
  const [error, setError] = useState(null);
  const [selectedName, setSelectedName] = useState('');
  const drawerRef = useRef(null);

  useEffect(() => {
    let cancelled = false;
    app.ProjectMemory(projectID)
      .then((p) => {
        if (cancelled) return;
        setPayload(p ?? null);
        setSelectedName((cur) => cur || (p?.memories?.[0]?.name ?? ''));
      })
      .catch((err) => { if (!cancelled) setError(err?.message ?? String(err)); });
    return () => { cancelled = true; };
  }, [projectID, app]);

  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  useEffect(() => { drawerRef.current?.focus(); }, []);

  const memories = payload?.memories ?? [];
  const skipped = payload?.skipped ?? [];
  const selected = memories.find((m) => m.name === selectedName) ?? null;

  // The three ways to have no memories are different states, and an empty list
  // says none of them: a store that cannot be read is not a store with nothing
  // in it.
  let meta = 'Loading…';
  if (error) meta = '';
  else if (payload?.unavailable) meta = `cannot be read: ${payload.unavailable}`;
  else if (payload) {
    const count = memories.length === 1 ? '1 memory' : `${memories.length} memories`;
    meta = `${count} · index ${payload.index_chars}/${payload.index_budget} characters sent on every call`;
  }

  return (
    <div
      ref={drawerRef}
      className="diff-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="Project memory"
      tabIndex={-1}
    >
      <div className="diff-drawer__header">
        <div>
          <h2 className="diff-drawer__title">Project Memory</h2>
          <p className="diff-drawer__meta">{meta}</p>
          {payload?.directory && (
            <p className="diff-drawer__meta">{payload.directory}</p>
          )}
        </div>
        <button type="button" className="diff-drawer__close" onClick={onClose} aria-label="Close">×</button>
      </div>

      {skipped.map((s) => (
        <p key={s.file} className="diff-drawer__error">
          {s.file} is skipped and never loaded: {s.reason}
        </p>
      ))}

      {error ? (
        <p className="diff-drawer__error">{error}</p>
      ) : payload && memories.length === 0 ? (
        <p className="diff-drawer__empty">
          Nothing is remembered for this project yet. The agent writes a memory when it
          decides something is worth carrying into later sessions.
        </p>
      ) : (
        <div className="diff-drawer__body">
          <aside className="diff-drawer__sidebar" aria-label="Memories">
            {memories.map((m) => (
              <button
                key={m.name}
                type="button"
                className={`diff-drawer__file ${selected?.name === m.name ? 'diff-drawer__file--active' : ''}`}
                onClick={() => setSelectedName(m.name)}
                title={m.description}
              >
                <span className="diff-drawer__file-path">
                  <span className="diff-drawer__file-name">{m.name}</span>
                  <span className="diff-drawer__file-dir">{m.type} · {m.description}</span>
                </span>
              </button>
            ))}
          </aside>

          <section className="diff-drawer__viewer" aria-label="Memory">
            {selected ? (
              <>
                <div className="diff-drawer__viewer-header">
                  <span className="diff-drawer__viewer-path">{selected.name}</span>
                  <span className="diff-drawer__viewer-kind">{selected.type}</span>
                  {formatWritten(selected) && (
                    <span className="diff-drawer__viewer-kind">{formatWritten(selected)}</span>
                  )}
                </div>
                <pre
                  className="diff-code"
                  style={{ whiteSpace: 'pre-wrap', overflow: 'auto', padding: '0.5rem' }}
                >
                  {selected.body}
                </pre>
              </>
            ) : (
              <p className="diff-drawer__empty">Select a memory to read it.</p>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
