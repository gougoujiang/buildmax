import { useEffect, useState } from 'react';

export function ApprovalPanel({ request, onRespond }) {
  const [selected, setSelected] = useState(0); // 0 = Allow, 1 = Deny

  useEffect(() => {
    function onKey(e) {
      switch (e.key) {
        case 'ArrowLeft':  setSelected(0); break;
        case 'ArrowRight': setSelected(1); break;
        case 'Enter':      onRespond(selected === 0); break;
        case 'y': case 'Y': onRespond(true); break;
        case 'n': case 'N': case 'Escape': onRespond(false); break;
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [selected, onRespond]);

  const argEntries = Object.entries(request.args ?? {});

  return (
    <div className="approval-panel">
      <div className="approval-panel__header">
        <span className="approval-panel__title">Tool Approval</span>
        <span className="approval-panel__tool">{request.tool_name}</span>
      </div>

      {argEntries.length > 0 && (
        <table className="approval-panel__args">
          <tbody>
            {argEntries.map(([k, v]) => {
              const val = String(v);
              const display = val.length > 80 ? val.slice(0, 77) + '…' : val;
              return (
                <tr key={k}>
                  <td className="approval-panel__arg-key">{k}</td>
                  <td className="approval-panel__arg-val">{display}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      <div className="approval-panel__footer">
        <button
          type="button"
          className={`approval-panel__btn ${selected === 0 ? 'approval-panel__btn--allow' : 'approval-panel__btn--muted'}`}
          onClick={() => onRespond(true)}
          onMouseEnter={() => setSelected(0)}
        >
          Allow(y)
        </button>
        <button
          type="button"
          className={`approval-panel__btn ${selected === 1 ? 'approval-panel__btn--deny' : 'approval-panel__btn--muted'}`}
          onClick={() => onRespond(false)}
          onMouseEnter={() => setSelected(1)}
        >
          Deny(n)
        </button>
        <span className="approval-panel__hint">← → select · Enter confirm</span>
      </div>
    </div>
  );
}

// --- ProjectItem ---
