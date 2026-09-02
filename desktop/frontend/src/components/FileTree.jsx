import { useTheme } from '@buildmax/gui';
import { useCallback, useEffect, useState } from 'react';
import { highlightToHtml } from '../lib/highlight';
import { usePaneResize } from '../lib/usePaneResize';
import { MarkdownMessage } from './MarkdownMessage';

const MARKDOWN_RE = /\.(md|markdown)$/i;

// FileTree browses a project's workspace in the inspector: a lazily expanded
// directory tree plus a preview of the selected file. Directories are listed
// one level at a time (App.ListWorkspaceDir) so a large repository is never
// walked up front; a clicked file is fetched on demand (App.ReadWorkspaceFile).
export function FileTree({ projectID, sessionID, app }) {
  const { theme } = useTheme();
  // dir path ("" is the root) -> { entries, error, loading }
  const [byDir, setByDir] = useState({});
  const [expanded, setExpanded] = useState(() => new Set(['']));
  // selected file path -> { content, binary, truncated, error, loading }
  const [selected, setSelected] = useState('');
  const [fileByPath, setFileByPath] = useState({});
  // 'source' (highlighted text) or 'preview' (rendered) — Markdown files only.
  const [viewMode, setViewMode] = useState('source');
  const [highlightedHtml, setHighlightedHtml] = useState(null);
  // Tree column width in the expanded (side-by-side) layout only.
  const { width: treeWidth, ref: browserRef, onMouseDown: startTreeResize } = usePaneResize('bm.desktop.fileTreeWidth', 260);

  const loadDir = useCallback((dir) => {
    if (!app?.ListWorkspaceDir) {
      setByDir((m) => ({ ...m, [dir]: { entries: [], error: 'Rebuild the desktop app to browse files.', loading: false } }));
      return;
    }
    setByDir((m) => ({ ...m, [dir]: { ...(m[dir] ?? {}), loading: true } }));
    app.ListWorkspaceDir(projectID, sessionID, dir)
      .then((res) => setByDir((m) => ({ ...m, [dir]: { entries: res?.entries ?? [], error: res?.error ?? null, loading: false } })))
      .catch((err) => setByDir((m) => ({ ...m, [dir]: { entries: [], error: err?.message ?? String(err), loading: false } })));
  }, [projectID, sessionID, app]);

  const selectFile = useCallback((filePath) => {
    setSelected(filePath);
    setViewMode('source');
    if (fileByPath[filePath]) return;
    if (!app?.ReadWorkspaceFile) {
      setFileByPath((m) => ({ ...m, [filePath]: { error: 'Rebuild the desktop app to preview files.', loading: false } }));
      return;
    }
    setFileByPath((m) => ({ ...m, [filePath]: { loading: true } }));
    app.ReadWorkspaceFile(projectID, sessionID, filePath)
      .then((res) => setFileByPath((m) => ({ ...m, [filePath]: { content: res?.content ?? '', binary: !!res?.binary, truncated: !!res?.truncated, error: res?.error ?? null, loading: false } })))
      .catch((err) => setFileByPath((m) => ({ ...m, [filePath]: { error: err?.message ?? String(err), loading: false } })));
  }, [projectID, sessionID, app, fileByPath]);

  // Reset and reload the root whenever the project or session changes — a
  // session may run in a worktree distinct from the project's default
  // workspace, so switching sessions can mean switching workspaces too.
  useEffect(() => {
    setByDir({});
    setExpanded(new Set(['']));
    setSelected('');
    setFileByPath({});
    setViewMode('source');
    loadDir('');
  }, [projectID, sessionID, loadDir]);

  const toggleDir = useCallback((dir) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(dir)) next.delete(dir);
      else {
        next.add(dir);
        if (!byDir[dir]) loadDir(dir);
      }
      return next;
    });
  }, [byDir, loadDir]);

  function renderEntries(dir, depth) {
    const node = byDir[dir];
    const pad = { paddingLeft: `${0.5 + depth * 0.85}rem` };
    if (!node || (node.loading && !node.entries)) {
      return <div className="file-tree__hint" style={pad}>Loading…</div>;
    }
    if (node.error) {
      return <div className="file-tree__hint file-tree__hint--error" style={pad}>{node.error}</div>;
    }
    if (node.entries.length === 0) {
      return <div className="file-tree__hint" style={pad}>Empty</div>;
    }
    return node.entries.map((e) => {
      const open = e.is_dir && expanded.has(e.path);
      const active = !e.is_dir && selected === e.path;
      return (
        <div key={e.path}>
          <div
            className={`file-tree__row ${e.is_dir ? 'file-tree__row--dir' : 'file-tree__row--file'} ${active ? 'file-tree__row--active' : ''}`}
            style={pad}
            onClick={e.is_dir ? () => toggleDir(e.path) : () => selectFile(e.path)}
            role="button"
            title={e.path}
          >
            <span className="file-tree__caret" aria-hidden>{e.is_dir ? (open ? '▾' : '▸') : ''}</span>
            <span className="file-tree__icon" aria-hidden>{e.is_dir ? '📁' : '📄'}</span>
            <span className="file-tree__name">{e.name}</span>
          </div>
          {open && renderEntries(e.path, depth + 1)}
        </div>
      );
    });
  }

  const file = selected ? fileByPath[selected] : null;
  const isMarkdown = MARKDOWN_RE.test(selected);

  // Highlights the current file's content whenever it, or the theme, changes.
  // Skipped in Markdown preview mode, which renders through MarkdownMessage
  // instead. Plain content renders immediately below and this swaps in
  // highlighted HTML once it resolves, so nothing blocks on it.
  useEffect(() => {
    let cancelled = false;
    if (!file || file.loading || file.error || file.binary || (isMarkdown && viewMode === 'preview')) {
      setHighlightedHtml(null);
      return undefined;
    }
    highlightToHtml(file.content, selected, theme)
      .then((html) => { if (!cancelled) setHighlightedHtml(html); })
      .catch(() => { if (!cancelled) setHighlightedHtml(null); });
    return () => { cancelled = true; };
  }, [selected, file, theme, isMarkdown, viewMode]);

  return (
    <div className="file-browser" ref={browserRef} style={{ '--tree-w': `${treeWidth}px` }}>
      <div className="file-browser__tree" aria-label="Workspace files">
        {renderEntries('', 0)}
      </div>
      <div
        className="file-browser__resizer"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize file tree"
        onMouseDown={startTreeResize}
      />
      <div className="file-browser__view" aria-label="File preview">
        {!selected ? (
          <p className="file-view__hint">Select a file to preview it.</p>
        ) : !file || file.loading ? (
          <p className="file-view__hint">Loading…</p>
        ) : file.error ? (
          <p className="file-view__hint file-view__hint--error">{file.error}</p>
        ) : file.binary ? (
          <p className="file-view__hint">Binary file — no preview.</p>
        ) : (
          <>
            <div className="file-view__header">
              <span className="file-view__path">{selected}</span>
              {file.truncated && <span className="file-view__badge">truncated</span>}
              {isMarkdown && (
                <div className="file-view__mode" role="group" aria-label="View mode">
                  <button
                    type="button"
                    className={`file-view__mode-btn ${viewMode === 'source' ? 'file-view__mode-btn--active' : ''}`}
                    aria-pressed={viewMode === 'source'}
                    onClick={() => setViewMode('source')}
                  >
                    Source
                  </button>
                  <button
                    type="button"
                    className={`file-view__mode-btn ${viewMode === 'preview' ? 'file-view__mode-btn--active' : ''}`}
                    aria-pressed={viewMode === 'preview'}
                    onClick={() => setViewMode('preview')}
                  >
                    Preview
                  </button>
                </div>
              )}
            </div>
            {isMarkdown && viewMode === 'preview' ? (
              <div className="file-view__preview">
                <MarkdownMessage content={file.content} />
              </div>
            ) : highlightedHtml ? (
              <div className="file-view__code" dangerouslySetInnerHTML={{ __html: highlightedHtml }} />
            ) : (
              <pre className="file-view__code">{file.content || ' '}</pre>
            )}
          </>
        )}
      </div>
    </div>
  );
}
