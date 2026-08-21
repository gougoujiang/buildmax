import { compareRecent, formatToolArgs, shortToolArgs, toolDisplayName } from './lib/format';
import { addLiveToolCall, addLiveToolResult, appendAssistantForNextLLM, buildToolResultMap, mergeRunStatus } from './lib/messages';
import { getApp } from './lib/app';
import { ChatInput } from './components/ChatInput';
import { HomeDashboard } from './components/HomeDashboard';
import { MarkdownMessage } from './components/MarkdownMessage';
import { CreateProjectModal } from './components/Modals';
import { ProjectItem } from './components/ProjectItem';

import { useState, useRef, useEffect, useMemo } from 'react';
import Markdown from 'react-markdown';
import { Avatar, ChatComposer, ChatThread, ThemeProvider, ThemeToggle } from '@buildmax/gui';
import { EventsOn, EventsOff } from './lib/wailsRuntime';
import LoginPage from './LoginPage';

export default function App() {
  const [sessions, setSessions] = useState([]);
  const [selectedId, setSelectedId] = useState(null);
  const [messages, setMessages] = useState([]);
  const [sessionTitle, setSessionTitle] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [wailsReady, setWailsReady] = useState(false);
  const [authStatus, setAuthStatus] = useState(null);

  const [projects, setProjects] = useState([]);
  const [projectsLoaded, setProjectsLoaded] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showAllProjects, setShowAllProjects] = useState(false);
  const [sessionFilter, setSessionFilter] = useState('');
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef(null);

  const PROJECT_PAGE_SIZE = 10;

  // The project for the next new chat (set when user clicks + on a project,
  // cleared once a session is created). For existing sessions the project is
  // derived from session.workspace.
  const [newChatProject, setNewChatProject] = useState(null);

  const historyRef = useRef(null);
  const streamingContentRef = useRef('');

  useEffect(() => {
    if (!userMenuOpen) return;
    function handleClickOutside(e) {
      if (userMenuRef.current && !userMenuRef.current.contains(e.target)) {
        setUserMenuOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [userMenuOpen]);

  // Group sessions by project ID (matched via folder_path).
  const sessionsByProject = useMemo(() => {
    const map = {};
    const q = sessionFilter.trim().toLowerCase();
    for (const proj of projects) {
      map[proj.id] = sessions
        .filter((s) => s.workspace === proj.folder_path)
        .filter((s) => !q ||
          (s.title ?? '').toLowerCase().includes(q) ||
          (s.id ?? '').toLowerCase().includes(q))
        .sort((a, b) => {
          if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1;
          return (b.created_at || '').localeCompare(a.created_at || '');
        });
    }
    return map;
  }, [projects, sessions, sessionFilter]);

  // The project currently in use: derived from the open session, or the pending
  // new-chat project when no session is selected yet.
  const currentProject = useMemo(() => {
    if (!selectedId) return newChatProject ?? null;
    const sess = sessions.find((s) => s.id === selectedId);
    if (!sess) return null;
    return projects.find((p) => p.folder_path === sess.workspace) ?? null;
  }, [selectedId, newChatProject, sessions, projects]);

  const projectByWorkspace = useMemo(() => {
    const map = new Map();
    for (const p of projects) {
      if (p.folder_path) map.set(p.folder_path, p);
    }
    return map;
  }, [projects]);

  const recentSessions = useMemo(() => {
    return [...sessions]
      .sort((a, b) => {
        if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1;
        return compareRecent(a.created_at, b.created_at);
      })
      .slice(0, 6);
  }, [sessions]);

  const recentProjects = useMemo(() => {
    return [...projects]
      .sort((a, b) => compareRecent(a.last_used_at || a.created_at, b.last_used_at || b.created_at))
      .slice(0, 6);
  }, [projects]);

  const EV_STREAM_DELTA = 'desktop/stream-delta';
  const EV_STREAM_DONE = 'desktop/stream-done';
  const EV_STREAM_ERROR = 'desktop/stream-error';
  const EV_APPROVAL_REQUEST = 'desktop/approval-request';
  const EV_LLM_START = 'desktop/llm-start';
  const EV_TOOL_START = 'desktop/tool-start';
  const EV_TOOL_END = 'desktop/tool-end';
  const EV_RUN_STATUS = 'desktop/run-status';
  const EV_MESSAGE_DEQUEUED = 'desktop/message-dequeued';
  const EV_MESSAGE_BLOCKED = 'desktop/message-blocked';

  const [approvalRequest, setApprovalRequest] = useState(null);
  const [toolActivity, setToolActivity] = useState('');
  const [runStatus, setRunStatus] = useState(null);
  // Prompts typed during a run, waiting for their own turn. The Go side owns the
  // real queue; this mirrors it so the transcript can show what is still waiting.
  const [queuedMessages, setQueuedMessages] = useState([]);

  useEffect(() => {
    const unsubDelta = EventsOn(EV_STREAM_DELTA, (delta) => {
      if (typeof delta !== 'string') return;
      streamingContentRef.current += delta;
      setMessages((prev) => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === 'assistant') {
          next[next.length - 1] = { ...last, content: streamingContentRef.current };
        }
        return next;
      });
    });
    const unsubDone = EventsOn(EV_STREAM_DONE, (payload) => {
      const reply = payload?.reply ?? streamingContentRef.current;
      const sessionId = payload?.session_id ?? '';
      streamingContentRef.current = '';
      setToolActivity('');
      setRunStatus({
        context_tokens: payload?.context_tokens ?? 0,
        context_window: payload?.context_window ?? 0,
        prompt_tokens: payload?.prompt_tokens ?? 0,
        completion_tokens: payload?.completion_tokens ?? 0,
        total_prompt_tokens: payload?.total_prompt_tokens ?? 0,
        total_completion_tokens: payload?.total_completion_tokens ?? 0,
      });
      setMessages((prev) => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === 'assistant') {
          // Cancellation before any model output: drop the empty placeholder
          // so the UI does not leave a blank assistant bubble behind.
          if (!reply) return next.slice(0, -1);
          next[next.length - 1] = { ...last, content: reply };
        }
        return next;
      });
      setNewChatProject(null);
      setSelectedId(sessionId || null);
      if (sessionId) {
        getApp()?.GetSession(sessionId).then((detail) => {
          setMessages(detail?.messages ?? []);
          setSessionTitle(detail?.title?.trim() || 'Chat');
        }).catch(() => {});
      }
      getApp()?.ListSessions().then((list) => setSessions(list ?? [])).catch(() => {});
      // The run goroutine only emits stream-done once the queue is drained.
      setQueuedMessages([]);
      setLoading(false);
    });
    const unsubError = EventsOn(EV_STREAM_ERROR, (payload) => {
      streamingContentRef.current = '';
      setToolActivity('');
      setError(payload?.message ?? 'Stream error');
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last?.role === 'assistant' && last?.content === '') return prev.slice(0, -1);
        return prev;
      });
      setLoading(false);
    });
    const unsubApproval = EventsOn(EV_APPROVAL_REQUEST, (payload) => {
      setApprovalRequest(payload);
    });
    const unsubLLMStart = EventsOn(EV_LLM_START, () => {
      streamingContentRef.current = '';
      setMessages((prev) => appendAssistantForNextLLM(prev));
    });
    const unsubToolStart = EventsOn(EV_TOOL_START, (payload) => {
      const name = payload?.tool_name ?? '';
      const args = payload?.args ? shortToolArgs(payload.args) : '';
      setToolActivity(args ? `⚙ ${name} (${args})` : `⚙ ${name}`);
      if (payload?.tool_call_id) {
        setMessages((prev) => addLiveToolCall(prev, {
          id: payload.tool_call_id,
          name,
          arguments: payload?.args || '',
        }));
      }
    });
    const unsubToolEnd = EventsOn(EV_TOOL_END, (payload) => {
      setToolActivity('');
      setMessages((prev) => addLiveToolResult(prev, payload));
    });
    const unsubRunStatus = EventsOn(EV_RUN_STATUS, (payload) => {
      setRunStatus((prev) => mergeRunStatus(prev, payload));
    });
    // A queued prompt starting its own turn: move it out of the waiting list and
    // into the transcript as a sent message, and keep the run indicator on.
    const unsubDequeued = EventsOn(EV_MESSAGE_DEQUEUED, (payload) => {
      setQueuedMessages(payload?.queued ?? []);
      const prompt = payload?.prompt ?? '';
      if (!prompt) return;
      setError(null);
      setLoading(true);
      streamingContentRef.current = '';
      setMessages((prev) => [...prev, { role: 'user', content: prompt }]);
    });
    // A hook refused a queued message. The run is still going, so this reports the
    // one message and leaves the loading state alone.
    const unsubBlocked = EventsOn(EV_MESSAGE_BLOCKED, (payload) => {
      setQueuedMessages(payload?.queued ?? []);
      setError(`Message blocked by hook: ${payload?.reason ?? 'no reason given'}`);
    });
    return () => {
      EventsOff(EV_STREAM_DELTA, EV_STREAM_DONE, EV_STREAM_ERROR, EV_APPROVAL_REQUEST, EV_LLM_START, EV_TOOL_START, EV_TOOL_END, EV_RUN_STATUS, EV_MESSAGE_DEQUEUED, EV_MESSAGE_BLOCKED);
      unsubDelta?.();
      unsubDone?.();
      unsubError?.();
      unsubApproval?.();
      unsubLLMStart?.();
      unsubToolStart?.();
      unsubToolEnd?.();
      unsubRunStatus?.();
      unsubDequeued?.();
      unsubBlocked?.();
    };
  }, []);

  // The queue lives per project on the Go side; re-read it when the visible
  // project changes so a queue built before a switch is still shown after it.
  useEffect(() => {
    const a = getApp();
    if (!a || !currentProject?.id || typeof a.QueuedMessages !== 'function') {
      setQueuedMessages([]);
      return;
    }
    let stale = false;
    a.QueuedMessages(currentProject.id)
      .then((list) => { if (!stale) setQueuedMessages(list ?? []); })
      .catch(() => {});
    return () => { stale = true; };
  }, [currentProject?.id]);

  useEffect(() => {
    if (getApp()) { setWailsReady(true); return; }
    const id = setTimeout(() => setWailsReady(true), 150);
    return () => clearTimeout(id);
  }, []);

  const app = getApp();

  useEffect(() => {
    if (!wailsReady || !app) return;
    app.GetAuthStatus()
      .then((status) => setAuthStatus(status))
      .catch(() => setAuthStatus({ logged_in: false }));
  }, [wailsReady, app]);

  useEffect(() => {
    if (!wailsReady || !app || !authStatus?.logged_in) return;
    Promise.all([app.ListProjects(), app.ListSessions()])
      .then(([list, sessionList]) => {
        setProjects(list ?? []);
        setSessions(sessionList ?? []);
        setProjectsLoaded(true);
      })
      .catch(() => setProjectsLoaded(true));
  }, [wailsReady, app, authStatus?.logged_in]);

  useEffect(() => {
    if (!wailsReady || !app || !currentProject) {
      setRunStatus(null);
      return;
    }
    app.GetRunStatus(currentProject.id, selectedId || '')
      .then((status) => setRunStatus(status ?? null))
      .catch(() => setRunStatus(null));
  }, [wailsReady, app, currentProject, selectedId]);

  function handleLogout() {
    if (!app) return;
    app.Logout()
      .then(() => setAuthStatus({ logged_in: false }))
      .catch(() => setAuthStatus({ logged_in: false }));
  }

  useEffect(() => {
    if (selectedId === null) {
      setMessages([]);
      setSessionTitle(newChatProject ? 'New Chat' : '');
      return;
    }
    if (!app) return;
    setError(null);
    app.GetSession(selectedId)
      .then((detail) => {
        setMessages(detail?.messages ?? []);
        setSessionTitle(detail?.title?.trim() || 'Chat');
      })
      .catch((err) => setError(err?.message ?? String(err)));
  }, [selectedId, app, newChatProject]);

  useEffect(() => {
    if (historyRef.current) {
      historyRef.current.scrollTop = historyRef.current.scrollHeight;
    }
  }, [messages]);

  function handleSelectSession(sessionId) {
    setNewChatProject(null);
    setSelectedId(sessionId);
  }

  function handleGoHome() {
    setNewChatProject(null);
    setSelectedId(null);
    setMessages([]);
    setSessionTitle('');
    setError(null);
  }

  function handleNewChatInProject(project) {
    setNewChatProject(project);
    setSelectedId(null);
    setMessages([]);
    setSessionTitle('New Chat');
  }

  async function handleCreateProject(name, folderPath) {
    try {
      const project = await app.CreateProject(name, folderPath);
      setProjects((prev) => [...prev, project]);
      // Open a new chat in the freshly created project.
      handleNewChatInProject(project);
      setShowCreateModal(false);
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleRenameProject(id, newName) {
    try {
      await app.RenameProject(id, newName);
      setProjects((prev) => prev.map((p) => p.id === id ? { ...p, name: newName } : p));
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleDeleteProject(id) {
    try {
      await app.DeleteProject(id);
      setProjects((prev) => prev.filter((p) => p.id !== id));
      // If the deleted project was in use, clear the chat area.
      if (currentProject?.id === id) {
        setNewChatProject(null);
        setSelectedId(null);
        setMessages([]);
      }
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleRenameSession(id, title) {
    try {
      await app.RenameSession(id, title);
      setSessions((prev) => prev.map((s) => s.id === id ? { ...s, title } : s));
      if (selectedId === id) setSessionTitle(title || 'Chat');
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleDeleteSession(id) {
    try {
      await app.DeleteSession(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
      if (selectedId === id) {
        setSelectedId(null);
        setMessages([]);
        setSessionTitle(newChatProject ? 'New Chat' : '');
      }
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handlePinSession(id, pinned) {
    try {
      await app.SetSessionPinned(id, pinned);
      setSessions((prev) => prev.map((s) => s.id === id ? { ...s, pinned } : s));
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleClearProjectSessions(project, projectSessions = []) {
    const ok = window.confirm(`Clear all sessions for ${project.name}? This does not delete files in the project folder.`);
    if (!ok) return;
    try {
      const visibleIds = projectSessions.map((s) => s.id);
      let deleted = [];
      if (typeof app.ClearProjectSessions === 'function') {
        deleted = await app.ClearProjectSessions(project.id);
      }
      if ((deleted ?? []).length === 0 && visibleIds.length > 0) {
        await Promise.all(visibleIds.map((id) => app.DeleteSession(id)));
        deleted = visibleIds;
      }
      const deletedSet = new Set(deleted ?? visibleIds);
      setSessions((prev) => prev.filter((s) => !deletedSet.has(s.id)));
      if (selectedId && deletedSet.has(selectedId)) {
        setSelectedId(null);
        setMessages([]);
        setSessionTitle('');
        if (currentProject?.id === project.id) setNewChatProject(project);
      }
    } catch (err) {
      setError(err?.message ?? String(err));
    }
  }

  async function handleRespond(approved) {
    if (!approvalRequest || !app) return;
    const req = approvalRequest;
    setApprovalRequest(null);
    try {
      await app.RespondApproval(req.project_id, approved);
    } catch (err) {
      console.error('RespondApproval failed:', err);
    }
  }

  async function handleSend(prompt) {
    if (!prompt?.trim() || !app || !currentProject) return;
    const trimmed = prompt.trim();

    // While a run is in flight the message is queued rather than refused. Ask the
    // Go side first: it owns the queue, and its answer says which of the two happened.
    if (loading) {
      try {
        const position = await app.SendMessageStream(currentProject.id, selectedId || '', trimmed);
        if (position > 0) setQueuedMessages((prev) => [...prev, trimmed]);
      } catch (err) {
        setError(err?.message ?? String(err));
      }
      return;
    }

    setLoading(true);
    setError(null);
    streamingContentRef.current = '';
    setRunStatus((prev) => ({ ...(prev ?? {}), prompt_tokens: 0, completion_tokens: 0 }));
    setMessages((prev) => [
      ...prev,
      { role: 'user', content: trimmed },
      { role: 'assistant', content: '' },
    ]);
    try {
      await app.SendMessageStream(currentProject.id, selectedId || '', trimmed);
    } catch (err) {
      setError(err?.message ?? String(err));
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last?.role === 'assistant' && last?.content === '') return prev.slice(0, -1);
        return prev;
      });
      setLoading(false);
    }
  }

  async function handleCancel() {
    if (!loading || !app || !currentProject) return;
    // Stopping discards the queue on both sides — see App.CancelRun.
    setQueuedMessages([]);
    try {
      await app.CancelRun(currentProject.id);
    } catch (err) {
      // Cancellation is best-effort; surface unexpected failures but do not
      // block UI state — the in-flight run will still complete via stream-done.
      console.error('CancelRun failed:', err);
    }
  }

  // --- Loading / auth screens ---

  if (!wailsReady) {
    return (
      <ThemeProvider>
        <div className="shell"><div className="shell__body" style={{ padding: '2rem' }}>
          <p className="page-chat__muted">Loading…</p>
        </div></div>
      </ThemeProvider>
    );
  }
  if (!app) {
    return (
      <ThemeProvider>
        <div className="shell"><div className="shell__body" style={{ padding: '2rem' }}>
          <p className="page-chat__muted">
            Run this app with Wails (e.g. <code>wails dev</code> or <code>./make run desktop</code>).
          </p>
        </div></div>
      </ThemeProvider>
    );
  }
  if (authStatus === null || (authStatus?.logged_in && !projectsLoaded)) {
    return (
      <ThemeProvider>
        <div className="shell"><div className="shell__body" style={{ padding: '2rem' }}>
          <p className="page-chat__muted">Loading…</p>
        </div></div>
      </ThemeProvider>
    );
  }
  if (!authStatus.logged_in) {
    return (
      <ThemeProvider>
        <LoginPage onLogin={(status) => setAuthStatus(status)} />
      </ThemeProvider>
    );
  }

  const toolResults = buildToolResultMap(messages);

  const threadItems = messages.flatMap((m, i) => {
    if (m.role === 'tool') return [];
    const toolCallLines = (m.tool_calls || []).map((tc, j) => {
      const result = toolResults.get(tc.id);
      const state = result ? (result.ok ? 'success' : 'error') : 'pending';
      const args = shortToolArgs(tc.arguments);
      return (
        <details key={tc.id || j} className={`page-chat__tool-call page-chat__tool-call--${state}`}>
          <summary>
            <span className="page-chat__tool-call-dot" aria-hidden />
            <span className="page-chat__tool-call-name">{toolDisplayName(tc.name)}</span>
            {args && <span className="page-chat__tool-call-args">({args})</span>}
          </summary>
          {tc.arguments && (
            <pre className="page-chat__tool-call-block">{formatToolArgs(tc.arguments)}</pre>
          )}
          {result?.content && (
            <div className="page-chat__tool-call-result">
              <MarkdownMessage content={result.content} />
            </div>
          )}
        </details>
      );
    });
    return [{
      id: `message-${i}`,
      role: m.role,
      label: m.role === 'user' ? 'You' : m.role,
      hideAvatar: true,
      body: (
        <div className="page-chat__msg-content">
          {m.content ? <MarkdownMessage content={m.content} /> : null}
          {toolCallLines}
        </div>
      ),
    }];
  });

  // Queued messages sit at the end of the transcript, dimmed: typed and accepted,
  // but not sent to the model yet.
  for (const [i, queued] of queuedMessages.entries()) {
    threadItems.push({
      id: `queued-${i}`,
      role: 'user',
      label: 'You (queued)',
      hideAvatar: true,
      body: (
        <div className="page-chat__msg-content page-chat__msg-content--queued">
          <MarkdownMessage content={queued} />
        </div>
      ),
    });
  }

  return (
    <ThemeProvider>
      <div className="shell">
        <div className="shell__body">

          <aside className="sidebar" aria-label="Sidebar">
            <nav className="sidebar__nav" aria-label="Primary">
              <div className="sidebar__projects-header">
                <span className="sidebar__projects-label">Projects</span>
                <button
                  type="button"
                  className="sidebar__projects-add"
                  onClick={() => setShowCreateModal(true)}
                  title="New Project"
                  aria-label="New Project"
                >
                  +
                </button>
              </div>
              <button
                type="button"
                className={`sidebar__home-btn ${!currentProject ? 'sidebar__home-btn--active' : ''}`}
                onClick={handleGoHome}
              >
                <span className="sidebar__home-icon" aria-hidden>⌂</span>
                <span>Home</span>
              </button>
              <div className="sidebar__session-search">
                <input
                  type="search"
                  className="sidebar__session-search-input"
                  value={sessionFilter}
                  onChange={(e) => setSessionFilter(e.target.value)}
                  placeholder="Search sessions"
                  aria-label="Search sessions"
                />
              </div>

              {projects.length === 0 ? (
                <button
                  type="button"
                  className="sidebar__nav-item sidebar__projects-empty-btn"
                  onClick={() => setShowCreateModal(true)}
                >
                  <span className="sidebar__nav-icon" aria-hidden>+</span>
                  <span>New Project</span>
                </button>
              ) : (
                <>
                  {(showAllProjects ? projects : projects.slice(0, PROJECT_PAGE_SIZE)).map((proj) => (
                    <ProjectItem
                      key={proj.id}
                      project={proj}
                      sessions={sessionsByProject[proj.id] ?? []}
                      isActive={currentProject?.id === proj.id}
                      selectedSessionId={selectedId}
                      onSelectSession={handleSelectSession}
                      onNewChat={() => handleNewChatInProject(proj)}
                      onRename={handleRenameProject}
                      onDelete={handleDeleteProject}
                      onClearSessions={(projectSessions) => handleClearProjectSessions(proj, projectSessions)}
                      onRenameSession={handleRenameSession}
                      onDeleteSession={handleDeleteSession}
                      onPinSession={handlePinSession}
                    />
                  ))}
                  {!showAllProjects && projects.length > PROJECT_PAGE_SIZE && (
                    <button
                      type="button"
                      className="sidebar__show-more"
                      onClick={() => setShowAllProjects(true)}
                    >
                      Show {projects.length - PROJECT_PAGE_SIZE} more…
                    </button>
                  )}
                </>
              )}
            </nav>

            <div className="sidebar__footer" ref={userMenuRef}>
              <button
                type="button"
                className="sidebar__user-trigger"
                onClick={() => setUserMenuOpen((v) => !v)}
                aria-expanded={userMenuOpen}
                aria-haspopup="menu"
                aria-label="User menu"
              >
                <Avatar
                  label={(authStatus.name?.trim() || authStatus.email || 'U').slice(0, 1).toUpperCase()}
                  size="sm"
                />
                <span className="sidebar__user-name">
                  {authStatus.name?.trim() || (authStatus.email ? authStatus.email.split('@')[0] : '')}
                </span>
              </button>
              {userMenuOpen && (
                <div className="sidebar__user-menu" role="menu">
                  <div className="sidebar__user-menu-email">{authStatus.email}</div>
                  <div className="sidebar__user-menu-divider" />
                  <button
                    type="button"
                    className="sidebar__user-menu-item"
                    role="menuitem"
                    onClick={() => { setUserMenuOpen(false); handleLogout(); }}
                  >
                    Sign out
                  </button>
                </div>
              )}
            </div>
          </aside>

          <main className="shell__main">
            <div className="shell__top">
              <div className="shell__top-titles">
                <span className="shell__title">
                  {currentProject ? (sessionTitle || 'New Chat') : 'Home'}
                </span>
                {currentProject && (
                  <span className="shell__subtitle">
                    {currentProject.name} · {currentProject.folder_path}
                  </span>
                )}
              </div>
              <ThemeToggle />
            </div>
            <div className="shell__content">
              {!currentProject ? (
                <HomeDashboard
                  recentSessions={recentSessions}
                  recentProjects={recentProjects}
                  projectByWorkspace={projectByWorkspace}
                  onSelectSession={handleSelectSession}
                  onOpenProject={handleNewChatInProject}
                  onCreateProject={() => setShowCreateModal(true)}
                />
              ) : (
                <div className="page-chat">
                  <ChatThread
                    historyRef={historyRef}
                    ariaLabel="Conversation history"
                    items={threadItems}
                    emptyText="Type a message below to start a new chat."
                  />
                  <section className="page-chat__input" aria-label="Send a message">
                    <ChatInput
                      onSend={handleSend}
                      onCancel={handleCancel}
                      loading={loading}
                      error={error}
                      onDismissError={() => setError(null)}
                      currentProject={currentProject}
                      app={app}
                      approvalRequest={approvalRequest}
                      onRespond={handleRespond}
                      toolActivity={toolActivity}
                      runStatus={runStatus}
                      sessionId={selectedId || ''}
                      onRunStatusContext={(status) => {
                        setRunStatus((prev) => ({
                          ...(status ?? {}),
                          prompt_tokens: prev?.prompt_tokens ?? 0,
                          completion_tokens: prev?.completion_tokens ?? 0,
                          total_prompt_tokens: prev?.total_prompt_tokens ?? status?.total_prompt_tokens ?? 0,
                          total_completion_tokens: prev?.total_completion_tokens ?? status?.total_completion_tokens ?? 0,
                        }));
                      }}
                    />
                  </section>
                </div>
              )}
            </div>
          </main>
        </div>
      </div>

      {showCreateModal && (
        <CreateProjectModal
          app={app}
          onCreate={handleCreateProject}
          onClose={() => setShowCreateModal(false)}
        />
      )}

    </ThemeProvider>
  );
}
