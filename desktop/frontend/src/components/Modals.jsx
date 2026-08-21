import { useEffect, useState } from 'react';
import { folderBaseName } from '../lib/format';

export function CreateProjectModal({ app, onCreate, onClose }) {
  const [name, setName] = useState('');
  const [folderPath, setFolderPath] = useState('');
  const [creating, setCreating] = useState(false);
  const [browseError, setBrowseError] = useState('');

  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  async function handleBrowse() {
    setBrowseError('');
    try {
      const path = await app.OpenFolderDialog();
      if (path) {
        setFolderPath(path);
        if (!name.trim()) {
          const base = folderBaseName(path);
          if (base) setName(base);
        }
      }
    } catch {
      setBrowseError('Could not open folder picker.');
    }
  }

  async function handleCreate() {
    const trimmedName = name.trim();
    if (!trimmedName || !folderPath || creating) return;
    setCreating(true);
    try {
      await onCreate(trimmedName, folderPath);
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="modal-overlay" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }} role="presentation">
      <div className="modal-panel" role="dialog" aria-modal="true" aria-label="New Project">
        <div className="modal-header">
          <h2 className="modal-title">New Project</h2>
          <button type="button" className="modal-close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="modal-body">
          <label className="modal-label" htmlFor="proj-name">Name</label>
          <input
            id="proj-name"
            className="modal-input"
            placeholder="My Project"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') folderPath ? handleCreate() : handleBrowse(); }}
            autoFocus
          />
          <label className="modal-label">Folder</label>
          <button type="button" className="modal-browse-btn" onClick={handleBrowse}>
            {folderPath
              ? <span className="modal-browse-path" title={folderPath}>{folderPath}</span>
              : <span className="modal-browse-placeholder">Choose folder…</span>}
          </button>
          {browseError && <p className="modal-field-error">{browseError}</p>}
        </div>
        <div className="modal-footer">
          <button type="button" className="modal-btn modal-btn--cancel" onClick={onClose}>Cancel</button>
          <button
            type="button"
            className="modal-btn modal-btn--primary"
            onClick={handleCreate}
            disabled={!name.trim() || !folderPath || creating}
          >
            {creating ? 'Creating…' : 'Create Project'}
          </button>
        </div>
      </div>
    </div>
  );
}

// --- Shared info modal ---

export function InfoModal({ title, onClose, children }) {
  useEffect(() => {
    function onKey(e) { if (e.key === 'Escape') onClose(); }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div
      className="modal-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      role="presentation"
    >
      <div className="modal-panel info-modal-panel" role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-header">
          <h2 className="modal-title">{title}</h2>
          <button type="button" className="modal-close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="info-modal-body">{children}</div>
      </div>
    </div>
  );
}

export function InfoList({ items, emptyText }) {
  if (!items) return <p className="info-modal__muted">Loading…</p>;
  if (!items.length) return <p className="info-modal__muted">{emptyText}</p>;
  return (
    <ul className="info-modal__list">
      {items.map((item, i) => (
        <li key={item.key ?? i} className={`info-modal__item ${item.active ? 'info-modal__item--active' : ''}`}>
          <div className="info-modal__item-row">
            <span className="info-modal__item-name">{item.name}</span>
            {item.badge && (
              <span className={`info-modal__badge info-modal__badge--${item.badgeVariant ?? 'default'}`}>
                {item.badge}
              </span>
            )}
          </div>
          {item.sub && <span className="info-modal__item-sub">{item.sub}</span>}
          {item.path && <span className="info-modal__item-path">{item.path}</span>}
        </li>
      ))}
    </ul>
  );
}

// --- MCP modal ---

export function MCPModal({ projectID, app, onClose }) {
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);
  useEffect(() => {
    app.GetSlashMCP(projectID)
      .then(setResult)
      .catch((err) => setError(err?.message ?? String(err)));
  }, [projectID, app]);

  let items = null;
  if (result) {
    if (result.load_error) {
      return (
        <InfoModal title="MCP Servers" onClose={onClose}>
          <p className="info-modal__error">{result.load_error}</p>
        </InfoModal>
      );
    }
    items = (result.servers ?? []).map((s) => ({
      key: s.id,
      name: s.id,
      badge: s.ok ? `${s.tool_count} tool${s.tool_count !== 1 ? 's' : ''}` : 'error',
      badgeVariant: s.ok ? 'ok' : 'err',
      sub: s.type + ((!s.ok && s.error) ? ` · ${s.error}` : ''),
    }));
  }
  return (
    <InfoModal title="MCP Servers" onClose={onClose}>
      {error
        ? <p className="info-modal__error">{error}</p>
        : <InfoList items={items} emptyText="No MCP servers configured." />}
    </InfoModal>
  );
}

// --- Agents modal ---

export function AgentsModal({ projectID, app, onClose }) {
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);
  useEffect(() => {
    app.GetSlashAgents(projectID)
      .then(setResult)
      .catch((err) => setError(err?.message ?? String(err)));
  }, [projectID, app]);

  const items = result
    ? (result.agents ?? []).map((a) => ({
        key: a.name,
        name: a.name,
        badge: a.is_builtin ? 'builtin' : 'custom',
        badgeVariant: a.is_builtin ? 'default' : 'ok',
        sub: a.description,
      }))
    : null;
  return (
    <InfoModal title="Agents" onClose={onClose}>
      {error
        ? <p className="info-modal__error">{error}</p>
        : <InfoList items={items} emptyText="No agents defined." />}
    </InfoModal>
  );
}

// --- Diff drawer ---
