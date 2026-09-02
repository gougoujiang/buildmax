import { useCallback, useEffect, useState } from 'react';

// FileTree browses a project's workspace in the inspector: a lazily expanded
// directory tree plus a preview of the selected file. Directories are listed
// one level at a time (App.ListWorkspaceDir) so a large repository is never
// walked up front; a clicked file is fetched on demand (App.ReadWorkspaceFile).
export function FileTree({ projectID, app }) {
  // dir path ("" is the root) -> { entries, error, loading }
  const [byDir, setByDir] = useState({});
  const [expanded, setExpanded] = useState(() => new Set(['']));
  // selected file path -> { content, binary, truncated, error, loading }
  const [selected, setSelected] = useState('');
  const [fileByPath, setFileByPath] = useState({});

  const loadDir = useCallback((dir) => {
    if (!app?.ListWorkspaceDir) {
      setByDir((m) => ({ ...m, [dir]: { entries: [], error: 'Rebuild the desktop app to browse files.', loading: false } }));
      return;
    }
    setByDir((m) => ({ ...m, [dir]: { ...(m[dir] ?? {}), loading: true } }));
    app.ListWorkspaceDir(projectID, dir)
      .then((res) => setByDir((m) => ({ ...m, [dir]: { entries: res?.entries ?? [], error: res?.error ?? null, loading: false } })))
      .catch((err) => setByDir((m) => ({ ...m, [dir]: { entries: [], error: err?.message ?? String(err), loading: false } })));
  }, [projectID, app]);

  const selectFile = useCallback((filePath) => {
    setSelected(filePath);
    if (fileByPath[filePath]) return;
    if (!app?.ReadWorkspaceFile) {
      setFileByPath((m) => ({ ...m, [filePath]: { error: 'Rebuild the desktop app to preview files.', loading: false } }));
      return;
    }
    setFileByPath((m) => ({ ...m, [filePath]: { loading: true } }));
    app.ReadWorkspaceFile(projectID, filePath)
      .then((res) => setFileByPath((m) => ({ ...m, [filePath]: { content: res?.content ?? '', binary: !!res?.binary, truncated: !!res?.truncated, error: res?.error ?? null, loading: false } })))
      .catch((err) => setFileByPath((m) => ({ ...m, [filePath]: { error: err?.message ?? String(err), loading: false } })));
  }, [projectID, app, fileByPath]);

  // Reset and reload the root whenever the project changes.
  useEffect(() => {
    setByDir({});
    setExpanded(new Set(['']));
    setSelected('');
    setFileByPath({});
    loadDir('');
  }, [projectID, loadDir]);

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

  return (
    <div className="file-browser">
      <div className="file-browser__tree" aria-label="Workspace files">
        {renderEntries('', 0)}
      </div>
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
            </div>
            <pre className="file-view__code">{file.content || ' '}</pre>
          </>
        )}
      </div>
    </div>
  );
}
