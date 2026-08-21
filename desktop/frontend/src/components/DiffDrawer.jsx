import { useEffect, useRef, useState } from 'react';
import { displayDiffPath, parsePatchLines, splitPathForDisplay, statusGlyph, statusTitle, truncateMiddleText } from '../lib/format';

export function DiffDrawer({ projectID, app, onClose }) {
  const [diff, setDiff] = useState(null);
  const [error, setError] = useState(null);
  const [selectedPath, setSelectedPath] = useState('');
  const [focusedPane, setFocusedPane] = useState('list');
  const drawerRef = useRef(null);

  useEffect(() => {
    let cancelled = false;
    setDiff(null);
    setError(null);
    app.GetWorkspaceDiff(projectID)
      .then((result) => {
        if (cancelled) return;
        setDiff(result);
        const first = result?.files?.[0]?.path ?? '';
        setSelectedPath(first);
      })
      .catch((err) => {
        if (!cancelled) setError(err?.message ?? String(err));
      });
    return () => { cancelled = true; };
  }, [projectID, app]);

  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const files = diff?.files ?? [];
  const selected = files.find((f) => f.path === selectedPath) ?? files[0] ?? null;

  useEffect(() => {
    drawerRef.current?.focus();
  }, []);

  function handleDrawerKeyDown(e) {
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      setFocusedPane('list');
      return;
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      setFocusedPane('content');
      return;
    }
    if (focusedPane !== 'list' || !files.length) return;
    const current = Math.max(0, files.findIndex((f) => f.path === selected?.path));
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedPath(files[Math.max(0, current - 1)].path);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedPath(files[Math.min(files.length - 1, current + 1)].path);
    }
  }

  return (
    <div
      ref={drawerRef}
      className="diff-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="Changed files"
      tabIndex={-1}
      onKeyDown={handleDrawerKeyDown}
    >
      <div className="diff-drawer__header">
        <div>
          <h2 className="diff-drawer__title">Changes</h2>
          <p className="diff-drawer__meta">
            {diff ? `${files.length} changed file${files.length === 1 ? '' : 's'}` : 'Loading…'}
          </p>
        </div>
        <button type="button" className="diff-drawer__close" onClick={onClose} aria-label="Close">×</button>
      </div>

      {error ? (
        <p className="diff-drawer__error">{error}</p>
      ) : diff?.error ? (
        <p className="diff-drawer__empty">{diff.error}</p>
      ) : diff && files.length === 0 ? (
        <p className="diff-drawer__empty">No changed files.</p>
      ) : (
        <div className="diff-drawer__body">
          <aside
            className={`diff-drawer__sidebar ${focusedPane === 'list' ? 'diff-drawer__pane--focused' : ''}`}
            aria-label="Changed files"
            onClick={() => setFocusedPane('list')}
          >
            {diff ? files.map((file) => {
              const parts = splitPathForDisplay(file.path);
              return (
                <button
                  key={`${file.status}:${file.path}`}
                  type="button"
                  className={`diff-drawer__file ${selected?.path === file.path ? 'diff-drawer__file--active' : ''}`}
                  onClick={() => setSelectedPath(file.path)}
                  title={displayDiffPath(file)}
                >
                  <span className={`diff-drawer__status diff-drawer__status--${file.status}`}>
                    {statusGlyph(file.status)}
                  </span>
                  <span className="diff-drawer__file-path">
                    {parts.dir && <span className="diff-drawer__file-dir">{truncateMiddleText(parts.dir, 28)}</span>}
                    <span className="diff-drawer__file-name">{truncateMiddleText(parts.name, 34)}</span>
                  </span>
                  {(file.additions > 0 || file.deletions > 0) && (
                    <span className="diff-drawer__counts">
                      +{file.additions} -{file.deletions}
                    </span>
                  )}
                </button>
              );
            }) : <p className="diff-drawer__empty">Loading…</p>}
          </aside>

          <section
            className={`diff-drawer__viewer ${focusedPane === 'content' ? 'diff-drawer__pane--focused' : ''}`}
            aria-label="Diff content"
            onClick={() => setFocusedPane('content')}
          >
            {selected ? (
              <>
                <div className="diff-drawer__viewer-header">
                  <span className={`diff-drawer__status diff-drawer__status--${selected.status}`}>
                    {statusGlyph(selected.status)}
                  </span>
                  <span className="diff-drawer__viewer-path">{displayDiffPath(selected)}</span>
                  <span className="diff-drawer__viewer-kind">{statusTitle(selected.status)}</span>
                </div>
                {selected.binary ? (
                  <p className="diff-drawer__empty">Binary file changed.</p>
                ) : selected.patch ? (
                  <div className="diff-code" role="table">
                    {parsePatchLines(selected.patch).map((row, idx) => (
                      <div key={idx} className={`diff-code__row diff-code__row--${row.kind}`} role="row">
                        <span className="diff-code__line" role="cell">{row.oldLine}</span>
                        <span className="diff-code__line" role="cell">{row.newLine}</span>
                        <code className="diff-code__text" role="cell">{row.text || ' '}</code>
                      </div>
                    ))}
                    {selected.truncated && (
                      <div className="diff-code__truncated">Diff truncated.</div>
                    )}
                  </div>
                ) : (
                  <p className="diff-drawer__empty">No text diff available.</p>
                )}
              </>
            ) : (
              <p className="diff-drawer__empty">Select a changed file.</p>
            )}
          </section>
        </div>
      )}
    </div>
  );
}

// --- Skills popup (slash command in the input) ---
