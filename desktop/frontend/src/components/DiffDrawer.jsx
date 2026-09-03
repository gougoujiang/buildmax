import { useTheme } from '@buildmax/gui';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { buildDiffTree, displayDiffPath, highlightDiffRows, parsePatchLines, splitPathForDisplay, statusGlyph, statusTitle, truncateMiddleText } from '../lib/format';
import { highlightToLines } from '../lib/highlight';
import { usePaneResize } from '../lib/usePaneResize';

const TREE_VIEW_KEY = 'bm.desktop.diffTreeView';

function readTreeMode() {
  try {
    return localStorage.getItem(TREE_VIEW_KEY) === 'tree';
  } catch {
    return false;
  }
}

// flattenVisibleFiles returns the changed-file objects a tree currently shows,
// in top-to-bottom order, so keyboard navigation matches what the eye sees:
// files inside a collapsed directory are skipped.
function flattenVisibleFiles(nodes, collapsed, out = []) {
  for (const node of nodes) {
    if (node.type === 'file') {
      out.push(node.file);
    } else if (!collapsed.has(node.path)) {
      flattenVisibleFiles(node.children, collapsed, out);
    }
  }
  return out;
}

// DiffPanel is the workspace-changes content of the inspector column: a
// changed-file list and the selected file's patch. It owns only the content —
// the inspector frame supplies the title, expand and close — so it renders no
// dialog chrome. Narrow, the two panes stack (list over patch); expanded, CSS
// lays them side by side.
export function DiffPanel({ projectID, sessionID, app }) {
  const { theme } = useTheme();
  const [diff, setDiff] = useState(null);
  const [error, setError] = useState(null);
  const [selectedPath, setSelectedPath] = useState('');
  const [focusedPane, setFocusedPane] = useState('list');
  const [highlightedRows, setHighlightedRows] = useState(null);
  const [treeMode, setTreeMode] = useState(readTreeMode);
  const [collapsed, setCollapsed] = useState(() => new Set());
  const rootRef = useRef(null);
  // File-list column width in the expanded (side-by-side) layout only.
  const { width: listWidth, ref: bodyRef, onMouseDown: startListResize } = usePaneResize('bm.desktop.diffListWidth', 260);

  useEffect(() => {
    let cancelled = false;
    setDiff(null);
    setError(null);
    app.GetWorkspaceDiff(projectID, sessionID)
      .then((result) => {
        if (cancelled) return;
        setDiff(result);
        setSelectedPath(result?.files?.[0]?.path ?? '');
      })
      .catch((err) => {
        if (!cancelled) setError(err?.message ?? String(err));
      });
    return () => { cancelled = true; };
  }, [projectID, sessionID, app]);

  const files = useMemo(() => diff?.files ?? [], [diff]);
  const selected = files.find((f) => f.path === selectedPath) ?? files[0] ?? null;

  const tree = useMemo(() => buildDiffTree(files), [files]);
  // Files in the order they are shown, so ↑/↓ tracks the visible layout: the
  // flat list in list mode, the expanded tree order in tree mode.
  const navFiles = useMemo(
    () => (treeMode ? flattenVisibleFiles(tree, collapsed) : files),
    [treeMode, tree, collapsed, files],
  );

  useEffect(() => {
    try { localStorage.setItem(TREE_VIEW_KEY, treeMode ? 'tree' : 'list'); } catch { /* ignore */ }
  }, [treeMode]);

  // A freshly loaded diff starts fully expanded; the collapse set is keyed by
  // path and would otherwise carry stale entries across workspaces.
  useEffect(() => { setCollapsed(new Set()); }, [diff]);

  const toggleDir = useCallback((path) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path); else next.add(path);
      return next;
    });
  }, []);

  // Highlighting is per-hunk (see highlightDiffRows), so it's recomputed
  // whenever the selected file or theme changes; parsePatchLines below always
  // renders immediately, and this swaps in tokenized rows once ready.
  useEffect(() => {
    let cancelled = false;
    setHighlightedRows(null);
    if (!selected || selected.binary || !selected.patch) return undefined;
    highlightDiffRows(parsePatchLines(selected.patch), selected.path, theme, highlightToLines)
      .then((rows) => { if (!cancelled) setHighlightedRows(rows); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [selected, theme]);

  // Arrow keys move within the file list once the panel has focus; Left/Right
  // move between the list and the patch. The panel is a focusable region, not a
  // dialog, so it never traps focus. In tree mode a directory is expanded by
  // clicking it; ↑/↓ still walks the visible files.
  function handleKeyDown(e) {
    if (e.key === 'ArrowLeft') { e.preventDefault(); setFocusedPane('list'); return; }
    if (e.key === 'ArrowRight') { e.preventDefault(); setFocusedPane('content'); return; }
    if (focusedPane !== 'list' || !navFiles.length) return;
    const current = Math.max(0, navFiles.findIndex((f) => f.path === selected?.path));
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedPath(navFiles[Math.max(0, current - 1)].path);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedPath(navFiles[Math.min(navFiles.length - 1, current + 1)].path);
    }
  }

  // One changed-file row, shared by both layouts. In the tree the containing
  // directories are already shown, so only the file's own name is rendered and
  // the row is indented by depth; the flat list shows the dir prefix instead.
  function renderFileRow(file, { name, depth } = {}) {
    const parts = splitPathForDisplay(file.path);
    return (
      <button
        key={`${file.status}:${file.path}`}
        type="button"
        className={`diff-drawer__file ${selected?.path === file.path ? 'diff-drawer__file--active' : ''}`}
        style={depth != null ? { paddingLeft: `${0.55 + depth * 0.85 + 0.95}rem` } : undefined}
        onClick={() => setSelectedPath(file.path)}
        title={displayDiffPath(file)}
      >
        <span className={`diff-drawer__status diff-drawer__status--${file.status}`}>
          {statusGlyph(file.status)}
        </span>
        <span className="diff-drawer__file-path">
          {name != null ? (
            <span className="diff-drawer__file-name">{truncateMiddleText(name, 40)}</span>
          ) : (
            <>
              {parts.dir && <span className="diff-drawer__file-dir">{truncateMiddleText(parts.dir, 28)}</span>}
              <span className="diff-drawer__file-name">{truncateMiddleText(parts.name, 34)}</span>
            </>
          )}
        </span>
        {(file.additions > 0 || file.deletions > 0) && (
          <span className="diff-drawer__counts">
            +{file.additions} -{file.deletions}
          </span>
        )}
      </button>
    );
  }

  function renderTreeNodes(nodes, depth) {
    return nodes.map((node) => {
      if (node.type === 'file') {
        return renderFileRow(node.file, { name: node.name, depth });
      }
      const isCollapsed = collapsed.has(node.path);
      return (
        <div key={`dir:${node.path}`}>
          <button
            type="button"
            className="diff-drawer__dir"
            style={{ paddingLeft: `${0.55 + depth * 0.85}rem` }}
            onClick={() => toggleDir(node.path)}
            title={node.path}
            aria-expanded={!isCollapsed}
          >
            <span className="diff-drawer__caret" aria-hidden>{isCollapsed ? '▸' : '▾'}</span>
            <span className="diff-drawer__folder" aria-hidden>📁</span>
            <span className="diff-drawer__dir-name">{node.name}</span>
          </button>
          {!isCollapsed && renderTreeNodes(node.children, depth + 1)}
        </div>
      );
    });
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
          <div className="diff-drawer__toolbar">
            <div className="diff-drawer__viewmode" role="group" aria-label="File list layout">
              <button
                type="button"
                className={`diff-drawer__viewmode-btn ${treeMode ? '' : 'diff-drawer__viewmode-btn--active'}`}
                aria-pressed={!treeMode}
                onClick={() => setTreeMode(false)}
              >
                List
              </button>
              <button
                type="button"
                className={`diff-drawer__viewmode-btn ${treeMode ? 'diff-drawer__viewmode-btn--active' : ''}`}
                aria-pressed={treeMode}
                onClick={() => setTreeMode(true)}
              >
                Tree
              </button>
            </div>
          </div>
          {treeMode
            ? renderTreeNodes(tree, 0)
            : files.map((file) => renderFileRow(file))}
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
                  {(highlightedRows ?? parsePatchLines(selected.patch)).map((row, idx) => (
                    <div key={idx} className={`diff-code__row diff-code__row--${row.kind}`} role="row">
                      <span className="diff-code__line" role="cell">{row.oldLine}</span>
                      <span className="diff-code__line" role="cell">{row.newLine}</span>
                      <code className="diff-code__text" role="cell">
                        {row.tokens
                          ? row.tokens.map((t, i) => <span key={i} style={{ color: t.color }}>{t.content}</span>)
                          // Line-content kinds carry a leading +/-/space diff marker that
                          // highlighted tokens never include (stripped before tokenizing);
                          // strip it here too so the text doesn't shift once tokens arrive.
                          : ((row.kind === 'add' || row.kind === 'del' || row.kind === 'context' ? row.text.slice(1) : row.text) || ' ')}
                      </code>
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
