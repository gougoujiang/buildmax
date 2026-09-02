import { useEffect, useRef, useState } from 'react';
import { displayDiffPath, parsePatchLines, splitPathForDisplay, statusGlyph, statusTitle, truncateMiddleText } from '../lib/format';
import { usePaneResize } from '../lib/usePaneResize';

// DiffPanel is the workspace-changes content of the inspector column: a
// changed-file list and the selected file's patch. It owns only the content —
// the inspector frame supplies the title, expand and close — so it renders no
// dialog chrome. Narrow, the two panes stack (list over patch); expanded, CSS
// lays them side by side.
export function DiffPanel({ projectID, app }) {
  const [diff, setDiff] = useState(null);
  const [error, setError] = useState(null);
  const [selectedPath, setSelectedPath] = useState('');
  const [focusedPane, setFocusedPane] = useState('list');
  const rootRef = useRef(null);
  // File-list column width in the expanded (side-by-side) layout only.
  const { width: listWidth, ref: bodyRef, onMouseDown: startListResize } = usePaneResize('bm.desktop.diffListWidth', 260);

  useEffect(() => {
    let cancelled = false;
    setDiff(null);
    setError(null);
    app.GetWorkspaceDiff(projectID)
      .then((result) => {
        if (cancelled) return;
        setDiff(result);
        setSelectedPath(result?.files?.[0]?.path ?? '');
      })
      .catch((err) => {
        if (!cancelled) setError(err?.message ?? String(err));
      });
    return () => { cancelled = true; };
  }, [projectID, app]);

  const files = diff?.files ?? [];
  const selected = files.find((f) => f.path === selectedPath) ?? files[0] ?? null;

  // Arrow keys move within the file list once the panel has focus; Left/Right
  // move between the list and the patch. The panel is a focusable region, not a
  // dialog, so it never traps focus.
  function handleKeyDown(e) {
    if (e.key === 'ArrowLeft') { e.preventDefault(); setFocusedPane('list'); return; }
    if (e.key === 'ArrowRight') { e.preventDefault(); setFocusedPane('content'); return; }
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

  if (error) return <p className="diff-drawer__error">{error}</p>;
  if (diff?.error) return <p className="diff-drawer__empty">{diff.error}</p>;
  if (!diff) return <p className="diff-drawer__empty">Loading…</p>;
  if (files.length === 0) return <p className="diff-drawer__empty">No uncommitted changes in this workspace.</p>;

  return (
    <div
      ref={rootRef}
      className="diff-panel"
      tabIndex={-1}
      onKeyDown={handleKeyDown}
      aria-label="Changed files"
    >
      <div className="diff-drawer__body" ref={bodyRef} style={{ '--list-w': `${listWidth}px` }}>
        <aside
          className={`diff-drawer__sidebar ${focusedPane === 'list' ? 'diff-drawer__pane--focused' : ''}`}
          aria-label="Changed files"
          onClick={() => setFocusedPane('list')}
        >
          {files.map((file) => {
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
          })}
        </aside>

        <div
          className="diff-panel__resizer"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize file list"
          onMouseDown={startListResize}
        />

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
    </div>
  );
}
