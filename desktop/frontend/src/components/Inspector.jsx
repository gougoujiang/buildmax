import { DiffPanel } from './DiffDrawer';
import { FileTree } from './FileTree';
import { InfoPanel } from './InfoPanel';

// Inspector is the docked right-hand column. It frames a content panel — the
// workspace files, the diff, or the session info — with a shared header
// carrying the title, an expand toggle (fills the main+inspector area for
// review), and a close. The view is chosen from the shell toolbar and by the
// /diff and /info commands, so the frame stays dumb: it renders whichever
// panel `view` names.
const TITLES = { files: 'Files', diff: 'Changes', info: 'Info' };

export function Inspector({ view, expanded, width, projectID, sessionID, projectName, workspace, app, onToggleExpand, onClose }) {
  return (
    <aside
      className={`inspector ${expanded ? 'inspector--expanded' : ''}`}
      style={width != null ? { width } : undefined}
      aria-label="Inspector"
    >
      <div className="inspector__head">
        <span className="inspector__title">{TITLES[view] ?? 'Inspector'}</span>
        <div className="inspector__actions">
          <button
            type="button"
            className="inspector__btn"
            onClick={onToggleExpand}
            title={expanded ? 'Restore width' : 'Expand'}
            aria-label={expanded ? 'Restore width' : 'Expand'}
            aria-pressed={expanded}
          >
            {expanded ? '⤡' : '⤢'}
          </button>
          <button
            type="button"
            className="inspector__btn"
            onClick={onClose}
            title="Close"
            aria-label="Close inspector"
          >
            ✕
          </button>
        </div>
      </div>
      <div className="inspector__body">
        {view === 'files' && <FileTree projectID={projectID} app={app} />}
        {view === 'diff' && <DiffPanel projectID={projectID} app={app} />}
        {view === 'info' && <InfoPanel projectID={projectID} sessionID={sessionID} projectName={projectName} workspace={workspace} app={app} />}
      </div>
    </aside>
  );
}
