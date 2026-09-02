import { useEffect, useRef, useState } from 'react';
import { ChatComposer } from '@buildmax/gui';
import { formatRunStatus } from '../lib/format';
import { ApprovalPanel } from './ApprovalPanel';
import { DiffDrawer } from './DiffDrawer';
import { JobsDrawer } from './JobsDrawer';
import { MemoryDrawer } from './MemoryDrawer';
import { HistoryModal } from './HistoryModal';
import { AgentsModal, MCPModal, PluginsModal } from './Modals';
import { EventsOn } from '../lib/wailsRuntime';

export function SkillsPopup({ skills, filter, selected, onSelect, onHighlight }) {
  const filtered = skills.filter(
    (s) => !filter || s.name.toLowerCase().includes(filter.toLowerCase()),
  );
  if (!filtered.length) {
    return (
      <div className="slash-popup">
        <div className="slash-popup__title">Skills</div>
        <p className="slash-popup__empty">
          {skills.length === 0 ? 'No skills found.' : 'No match.'}
        </p>
      </div>
    );
  }
  return (
    <div className="slash-popup" role="listbox" aria-label="Skills">
      <div className="slash-popup__title">Skills — select to run</div>
      {filtered.map((s, i) => (
        <button
          key={s.name}
          type="button"
          role="option"
          aria-selected={i === selected}
          className={`slash-popup__item ${i === selected ? 'slash-popup__item--active' : ''}`}
          onMouseEnter={() => onHighlight(i)}
          onClick={() => onSelect(s)}
        >
          <span className="slash-popup__cmd">{s.name}</span>
          {s.description && <span className="slash-popup__desc">{s.description}</span>}
        </button>
      ))}
    </div>
  );
}

// --- ChatInput ---

export function ChatInput({ onSend, onCancel, loading, error, onDismissError, currentProject, app, approvalRequest, onRespond, toolActivity, runStatus, sessionId, onRunStatusContext, onRewound, onForked, suggestion, onAcceptSuggestion }) {
  const [prompt, setPrompt] = useState('');

  // Skills popup state
  const [skills, setSkills] = useState([]);
  const [skillsSelected, setSkillsSelected] = useState(0);

  // Status bar state (loaded per project)
  const [models, setModels] = useState([]);
  // Where this session's prompts go. It is the app's mode rather than a property
  // of one model, so the picker says it once above the list.
  const [modelMode, setModelMode] = useState('');
  const [currentModel, setCurrentModel] = useState('');
  const [gitBranch, setGitBranch] = useState('');
  const [showModelDropdown, setShowModelDropdown] = useState(false);
  const modelDropdownRef = useRef(null);

  // Lazy-loaded panel state
  const [showMCP, setShowMCP] = useState(false);
  const [showAgents, setShowAgents] = useState(false);
  const [showPlugins, setShowPlugins] = useState(false);
  const [showDiff, setShowDiff] = useState(false);
  const [showJobs, setShowJobs] = useState(false);
  const [showMemory, setShowMemory] = useState(false);
  const [showHistory, setShowHistory] = useState(false);

  // Count of running background jobs for the status-bar badge. The Go side
  // owns job state; this refreshes from it on every lifecycle event.
  const [runningJobs, setRunningJobs] = useState(0);
  useEffect(() => {
    if (!currentProject || !app?.ListJobs) return undefined;
    let cancelled = false;
    const refresh = () => {
      app.ListJobs(currentProject.id)
        .then((list) => { if (!cancelled) setRunningJobs((list ?? []).filter((j) => j.running).length); })
        .catch(() => {});
    };
    refresh();
    const unsub = EventsOn('desktop/job-update', (payload) => {
      if (payload?.project_id === currentProject.id) refresh();
    });
    return () => { cancelled = true; unsub(); };
  }, [currentProject, app]);

  // Load project-level data when project changes.
  useEffect(() => {
    if (!currentProject || !app) return;
    setModels([]);
    setCurrentModel('');
    setGitBranch('');
    setSkills([]);
    Promise.allSettled([
      app.GetSlashModels(currentProject.id),
      app.GetSlashSkills(currentProject.id),
      app.GetGitBranch(currentProject.id),
    ]).then(([modelsRes, skillsRes, branchRes]) => {
      if (modelsRes.status === 'fulfilled') {
        setModels(modelsRes.value.models ?? []);
        setCurrentModel(modelsRes.value.current ?? '');
        setModelMode(
          modelsRes.value.managed
            ? `Prompts go to ${modelsRes.value.server_url}`
            : 'Prompts go from this machine to each provider',
        );
      }
      if (skillsRes.status === 'fulfilled') setSkills(skillsRes.value.skills ?? []);
      if (branchRes.status === 'fulfilled') setGitBranch(branchRes.value ?? '');
    });
  }, [currentProject, app]);

  // Close model dropdown on outside click.
  useEffect(() => {
    if (!showModelDropdown) return;
    function onOutside(e) {
      if (modelDropdownRef.current && !modelDropdownRef.current.contains(e.target)) {
        setShowModelDropdown(false);
      }
    }
    document.addEventListener('mousedown', onOutside);
    return () => document.removeEventListener('mousedown', onOutside);
  }, [showModelDropdown]);

  // Skills popup: trigger on "/" prefix; text after "/" is the filter.
  const skillsFilter = (() => {
    const first = prompt.split('\n')[0].trimStart();
    if (!first.startsWith('/')) return null;
    const after = first.slice(1);
    // Hide once a space appears — user has finished the skill name and is typing args.
    if (after.includes(' ')) return null;
    return after;
  })();
  const showSkillsPopup = skillsFilter !== null;
  const filteredSkills = showSkillsPopup
    ? skills.filter((s) => !skillsFilter || s.name.toLowerCase().includes(skillsFilter.toLowerCase()))
    : [];

  function handleSubmit() {
    const value = prompt.trim();
    if (!value) return;
    // No loading guard: onSend queues the message when a run is in flight.
    onSend(value);
    setPrompt('');
  }

  function handleChange(value) {
    setPrompt(value);
    onDismissError();
    setSkillsSelected(0);
  }

  // Accepting puts the suggestion in the box for the user to send or edit; it
  // is never sent on their behalf.
  function handleAcceptSuggestion() {
    setPrompt(suggestion);
    onAcceptSuggestion?.();
  }

  function handleSelectSkill(skill) {
    setPrompt(`/${skill.name}`);
    setSkillsSelected(0);
  }

  async function handleModelSwitch(modelName) {
    setShowModelDropdown(false);
    try {
      await app.SetProjectModel(currentProject.id, modelName);
      setCurrentModel(modelName);
      const status = await app.GetRunStatus(currentProject.id, sessionId || '');
      onRunStatusContext?.(status);
    } catch { /* ignore */ }
  }

  function handleKeyDown(e) {
    if (!showSkillsPopup || !filteredSkills.length) return;
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSkillsSelected((p) => (p - 1 + filteredSkills.length) % filteredSkills.length);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSkillsSelected((p) => (p + 1) % filteredSkills.length);
    } else if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      const skill = filteredSkills[skillsSelected];
      if (skill) handleSelectSkill(skill);
    } else if (e.key === 'Escape') {
      setPrompt('');
    }
  }

  const displayModel = currentModel
    ? (currentModel.length > 24 ? currentModel.slice(0, 22) + '…' : currentModel)
    : 'No model';

  return (
    <div className="chat-input-wrap">
      {approvalRequest && (
        <ApprovalPanel request={approvalRequest} onRespond={onRespond} />
      )}

      {toolActivity && (
        <div className="tool-activity-bar">{toolActivity}</div>
      )}

      {showSkillsPopup && (
        <SkillsPopup
          skills={skills}
          filter={skillsFilter}
          selected={skillsSelected}
          onSelect={handleSelectSkill}
          onHighlight={setSkillsSelected}
        />
      )}

      <ChatComposer
        value={prompt}
        onChange={handleChange}
        onSubmit={handleSubmit}
        onCancel={onCancel}
        loading={loading}
        error={error}
        placeholder="Type a message… (/ for skills, Enter to send)"
        queueWhileLoading
        queuePlaceholder="Type a message… (Enter to queue it for the next turn)"
        ariaLabel="Message"
        onKeyDown={handleKeyDown}
        ghost={suggestion}
        onAcceptGhost={handleAcceptSuggestion}
      />

      <div className="chat-status-bar">
        {/* Model dropdown */}
        <div className="model-selector" ref={modelDropdownRef}>
          <button
            type="button"
            className="model-selector__btn"
            onClick={() => setShowModelDropdown((v) => !v)}
            title={currentModel || 'Select model'}
          >
            <span className="model-selector__label">{displayModel}</span>
            <span className="model-selector__arrow" aria-hidden>▾</span>
          </button>
          {showModelDropdown && models.length > 0 && (
            <div className="model-selector__dropdown" role="listbox">
              {modelMode && (
                <div className="model-selector__mode">{modelMode}</div>
              )}
              {models.map((m) => (
                <button
                  key={m.name}
                  type="button"
                  role="option"
                  aria-selected={m.is_current}
                  className={`model-selector__option ${m.is_current ? 'model-selector__option--active' : ''}`}
                  onClick={() => handleModelSwitch(m.name)}
                >
                  <span className="model-selector__option-name">{m.name}</span>
                  {m.provider_model && m.provider_model !== m.name && (
                    <span className="model-selector__option-sub">{m.provider_model}</span>
                  )}
                  {m.destination && (
                    <span className="model-selector__option-sub">{m.destination}</span>
                  )}
                  {m.is_current && <span className="model-selector__option-check" aria-hidden>✓</span>}
                </button>
              ))}
            </div>
          )}
        </div>

        <span className="chat-status-bar__run">
          {formatRunStatus(runStatus)}
        </span>

        {/* Git branch */}
        {gitBranch && (
          <span className="chat-status-bar__branch" title={`Branch: ${gitBranch}`}>
            ⎇ {gitBranch}
          </span>
        )}

        <div className="chat-status-bar__spacer" />

        {/* MCP button */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowDiff(true)}
          title="Changed files"
        >
          Diff
        </button>

        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowMCP(true)}
          title="MCP server status"
        >
          MCP
        </button>

        {/* Agents button */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowAgents(true)}
          title="Available agents"
        >
          Agents
        </button>

        {/* Plugins button */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowPlugins(true)}
          title="Installed plugins"
        >
          Plugins
        </button>

        {/* History button: rewind and fork both act on a saved session, so
            there is nothing to offer until this chat has one. */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowHistory(true)}
          disabled={!sessionId}
          title={sessionId ? 'Rewind or fork this conversation' : 'Send a message first'}
        >
          History
        </button>

        {/* Memory button */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowMemory(true)}
          title="What this project remembers across sessions"
        >
          Memory
        </button>

        {/* Jobs button */}
        <button
          type="button"
          className="chat-status-bar__btn"
          onClick={() => setShowJobs(true)}
          title="Background jobs"
        >
          {runningJobs > 0 ? `Jobs (${runningJobs})` : 'Jobs'}
        </button>
      </div>

      {showMCP && (
        <MCPModal projectID={currentProject.id} app={app} onClose={() => setShowMCP(false)} />
      )}
      {showAgents && (
        <AgentsModal projectID={currentProject.id} app={app} onClose={() => setShowAgents(false)} />
      )}
      {showPlugins && (
        <PluginsModal projectID={currentProject.id} app={app} onClose={() => setShowPlugins(false)} />
      )}
      {showDiff && (
        <DiffDrawer projectID={currentProject.id} app={app} onClose={() => setShowDiff(false)} />
      )}
      {showMemory && (
        <MemoryDrawer projectID={currentProject.id} app={app} onClose={() => setShowMemory(false)} />
      )}
      {showJobs && (
        <JobsDrawer projectID={currentProject.id} app={app} onClose={() => setShowJobs(false)} />
      )}
      {showHistory && sessionId && (
        <HistoryModal
          projectID={currentProject.id}
          sessionID={sessionId}
          app={app}
          draft={prompt}
          onRewound={(report, restored) => {
            // The rewound prompt comes back here to be edited and sent again;
            // the modal decides whether it may, since a draft already in the
            // composer wins.
            if (restored) setPrompt(restored);
            onRewound(report);
          }}
          onForked={onForked}
          onClose={() => setShowHistory(false)}
        />
      )}
    </div>
  );
}
