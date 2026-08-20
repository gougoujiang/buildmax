import { useState, useRef, useEffect, useMemo } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Avatar, ChatComposer, ChatThread, ThemeProvider, ThemeToggle } from '@buildmax/gui';
import { EventsOn, EventsOff } from './lib/wailsRuntime';
import LoginPage from './LoginPage';

function getApp() {
  if (typeof window === 'undefined') return null;
  const go = window.go;
  if (!go) return null;
  return go.desktop?.App ?? go.main?.App ?? go.App ?? null;
}

function formatSessionMeta(createdAt) {
  if (!createdAt) return '';
  try {
    const d = new Date(createdAt);
    const now = new Date();
    const diffMs = now - d;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m`;
    if (diffHours < 24) return `${diffHours}h`;
    if (diffDays < 7) return `${diffDays}d`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  } catch {
    return '';
  }
}

function compareRecent(a, b) {
  return String(b || '').localeCompare(String(a || ''));
}

function folderBaseName(path) {
  if (!path) return '';
  return path.split(/[/\\]/).filter(Boolean).pop() ?? path;
}

function formatToolArgs(raw) {
  if (!raw) return '';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

function toolDisplayName(name) {
  const first = String(name || '').split('_', 1)[0];
  if (!first) return String(name || '');
  return first.slice(0, 1).toUpperCase() + first.slice(1);
}

function buildToolResultMap(messages) {
  const map = new Map();
  for (const m of messages) {
    if (m.role === 'tool' && m.tool_call_id) {
      map.set(m.tool_call_id, {
        ok: !String(m.content || '').startsWith('error:'),
        content: m.content || '',
      });
    }
  }
  return map;
}

function appendAssistantForNextLLM(messages) {
  const last = messages[messages.length - 1];
  if (!last) return messages;
  if (last.role === 'assistant' && !last.content && !(last.tool_calls || []).length) {
    return messages;
  }
  if (last.role === 'user') return [...messages, { role: 'assistant', content: '' }];
  return [...messages, { role: 'assistant', content: '' }];
}

function addLiveToolCall(messages, call) {
  const next = [...messages];
  for (let i = next.length - 1; i >= 0; i -= 1) {
    if (next[i]?.role !== 'assistant') continue;
    const calls = next[i].tool_calls || [];
    if (calls.some((tc) => tc.id === call.id)) return messages;
    next[i] = { ...next[i], tool_calls: [...calls, call] };
    return next;
  }
  return [...messages, { role: 'assistant', content: '', tool_calls: [call] }];
}

function addLiveToolResult(messages, payload) {
  const id = payload?.tool_call_id;
  if (!id || messages.some((m) => m.role === 'tool' && m.tool_call_id === id)) {
    return messages;
  }
  const content = payload?.is_error
    ? `error: ${payload?.reason || 'tool call failed'}`
    : '';
  return [...messages, { role: 'tool', tool_call_id: id, content }];
}

function shortToolArgs(raw) {
  if (!raw) return '';
  if (raw.length <= 40) return raw;
  try {
    const m = JSON.parse(raw);
    for (const k of ['path', 'file', 'filename', 'command']) {
      if (typeof m[k] === 'string') {
        const v = m[k];
        return v.length > 40 ? v.slice(0, 37) + '…' : v;
      }
    }
    for (const v of Object.values(m)) {
      if (typeof v === 'string') return v.length > 40 ? v.slice(0, 37) + '…' : v;
    }
  } catch {
    // Not JSON — fall through to the raw truncation below.
  }
  return raw.slice(0, 37) + '…';
}

function formatTokenCount(n) {
  const value = Number(n) || 0;
  if (value < 1000) return String(value);
  if (value % 1000 === 0) return `${value / 1000}k`;
  return `${(value / 1000).toFixed(1)}k`;
}

function formatRunStatus(status) {
  const ctxTokens = Number(status?.context_tokens) || 0;
  const ctxWindow = Number(status?.context_window) || 0;
  const input = Number(status?.prompt_tokens) || 0;
  const output = Number(status?.completion_tokens) || 0;
  const totalInput = Number(status?.total_prompt_tokens) || 0;
  const totalOutput = Number(status?.total_completion_tokens) || 0;
  const ctx = ctxWindow > 0
    ? `ctx: ${Math.round((ctxTokens / ctxWindow) * 100)}% (${formatTokenCount(ctxTokens)}/${formatTokenCount(ctxWindow)})`
    : 'ctx: unknown';
  const totals = totalInput > 0 || totalOutput > 0
    ? ` (${formatTokenCount(totalInput)}/${formatTokenCount(totalOutput)})`
    : '';
  return `${ctx} | tokens(in/out): ${formatTokenCount(input)}/${formatTokenCount(output)}${totals}`;
}

function mergeRunStatus(prev, payload) {
  const next = { ...(prev ?? {}), ...(payload ?? {}) };
  const prevPrompt = Number(prev?.prompt_tokens) || 0;
  const prevCompletion = Number(prev?.completion_tokens) || 0;
  const nextPrompt = Number(payload?.prompt_tokens) || 0;
  const nextCompletion = Number(payload?.completion_tokens) || 0;
  if (payload?.total_prompt_tokens == null) {
    next.total_prompt_tokens = Number(prev?.total_prompt_tokens) || 0;
    if (nextPrompt > prevPrompt) next.total_prompt_tokens += nextPrompt - prevPrompt;
  }
  if (payload?.total_completion_tokens == null) {
    next.total_completion_tokens = Number(prev?.total_completion_tokens) || 0;
    if (nextCompletion > prevCompletion) next.total_completion_tokens += nextCompletion - prevCompletion;
  }
  return next;
}

function statusGlyph(status) {
  switch (status) {
    case 'added': return '+';
    case 'deleted': return '-';
    case 'renamed': return '↔';
    default: return '●';
  }
}

function statusTitle(status) {
  switch (status) {
    case 'added': return 'Added';
    case 'deleted': return 'Deleted';
    case 'renamed': return 'Renamed';
    default: return 'Modified';
  }
}

function displayDiffPath(file) {
  if (file?.status === 'renamed' && file.old_path) return `${file.old_path} → ${file.path}`;
  return file?.path ?? '';
}

function splitPathForDisplay(path) {
  const value = String(path ?? '');
  const parts = value.split(/[/\\]/);
  const name = parts.pop() || value;
  const dir = parts.length ? `${parts.join('/')}/` : '';
  return { dir, name };
}

function truncateMiddleText(value, max = 34) {
  const text = String(value ?? '');
  if (text.length <= max) return text;
  if (max <= 1) return '…';
  const head = Math.floor((max - 1) / 2);
  const tail = max - 1 - head;
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

function parseRangeStart(token) {
  const n = Number.parseInt(token.replace(/^[-+]/, '').split(',')[0], 10);
  return Number.isFinite(n) ? n : 0;
}

function parsePatchLines(patch) {
  const rows = [];
  let oldLine = 0;
  let newLine = 0;
  for (const raw of String(patch ?? '').replace(/\r\n/g, '\n').split('\n')) {
    if (raw.startsWith('@@')) {
      const parts = raw.split(/\s+/);
      oldLine = parseRangeStart(parts.find((p) => p.startsWith('-')) ?? '');
      newLine = parseRangeStart(parts.find((p) => p.startsWith('+')) ?? '');
      rows.push({ kind: 'hunk', text: raw, oldLine: '', newLine: '' });
      continue;
    }
    if (raw.startsWith('diff --git') || raw.startsWith('index ') || raw.startsWith('--- ') || raw.startsWith('+++ ')) {
      rows.push({ kind: 'header', text: raw, oldLine: '', newLine: '' });
      continue;
    }
    if (raw.startsWith('+')) {
      rows.push({ kind: 'add', text: raw, oldLine: '', newLine: newLine || '' });
      newLine += 1;
      continue;
    }
    if (raw.startsWith('-')) {
      rows.push({ kind: 'del', text: raw, oldLine: oldLine || '', newLine: '' });
      oldLine += 1;
      continue;
    }
    rows.push({ kind: 'context', text: raw, oldLine: oldLine || '', newLine: newLine || '' });
    if (oldLine) oldLine += 1;
    if (newLine) newLine += 1;
  }
  return rows;
}

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

function MarkdownMessage({ content }) {
  return (
    <div className="page-chat__markdown">
      <Markdown remarkPlugins={[remarkGfm]}>{content}</Markdown>
    </div>
  );
}

function HomeDashboard({ recentSessions, recentProjects, projectByWorkspace, onSelectSession, onOpenProject, onCreateProject }) {
  return (
    <div className="page-home">
      <div className="page-home__header">
        <div>
          <h1 className="page-home__title">Continue your work</h1>
          <p className="page-home__subtitle">Pick up a recent chat or open a project workspace.</p>
        </div>
        <button type="button" className="page-home__primary" onClick={onCreateProject}>
          New Project
        </button>
      </div>

      <div className="page-home__grid">
        <section className="page-home__section" aria-label="Recent chats">
          <div className="page-home__section-head">
            <h2>Recent chats</h2>
          </div>
          {recentSessions.length === 0 ? (
            <p className="page-home__empty">No recent chats yet.</p>
          ) : (
            <div className="page-home__list">
              {recentSessions.map((s) => {
                const project = projectByWorkspace.get(s.workspace);
                return (
                  <button
                    key={s.id}
                    type="button"
                    className="page-home__item"
                    onClick={() => onSelectSession(s.id)}
                  >
                    <span className="page-home__item-title">{s.pinned ? '★ ' : ''}{s.title?.trim() || 'Chat'}</span>
                    <span className="page-home__item-meta">
                      {project?.name || folderBaseName(s.workspace) || 'Unknown project'} · {formatSessionMeta(s.created_at)}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </section>

        <section className="page-home__section" aria-label="Recent projects">
          <div className="page-home__section-head">
            <h2>Recent projects</h2>
          </div>
          {recentProjects.length === 0 ? (
            <p className="page-home__empty">Create a project once, then it will stay here for quick access.</p>
          ) : (
            <div className="page-home__list">
              {recentProjects.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  className="page-home__item"
                  onClick={() => onOpenProject(p)}
                >
                  <span className="page-home__item-title">{p.name}</span>
                  <span className="page-home__item-meta">{p.folder_path}</span>
                </button>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

// --- ApprovalPanel ---

function ApprovalPanel({ request, onRespond }) {
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

const SESSION_PAGE_SIZE = 10;

function ProjectItem({ project, sessions, isActive, selectedSessionId, onSelectSession, onNewChat, onRename, onDelete, onClearSessions, onRenameSession, onDeleteSession, onPinSession }) {
  const [expanded, setExpanded] = useState(isActive);
  const [showMenu, setShowMenu] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(project.name);
  const [showAllSessions, setShowAllSessions] = useState(false);
  const [sessionMenuId, setSessionMenuId] = useState(null);
  const [renamingSessionId, setRenamingSessionId] = useState(null);
  const [sessionRenameValue, setSessionRenameValue] = useState('');
  const menuRef = useRef(null);
  const sessionMenuRef = useRef(null);

  useEffect(() => { if (isActive) setExpanded(true); }, [isActive]);
  useEffect(() => { setRenameValue(project.name); }, [project.name]);

  useEffect(() => {
    if (!showMenu) return;
    function onOutside(e) {
      if (menuRef.current && !menuRef.current.contains(e.target)) setShowMenu(false);
    }
    document.addEventListener('mousedown', onOutside);
    return () => document.removeEventListener('mousedown', onOutside);
  }, [showMenu]);

  useEffect(() => {
    if (!sessionMenuId) return;
    function onOutside(e) {
      if (sessionMenuRef.current && !sessionMenuRef.current.contains(e.target)) setSessionMenuId(null);
    }
    document.addEventListener('mousedown', onOutside);
    return () => document.removeEventListener('mousedown', onOutside);
  }, [sessionMenuId]);

  function startRename() {
    setShowMenu(false);
    setRenaming(true);
    setRenameValue(project.name);
  }

  async function submitRename() {
    const trimmed = renameValue.trim();
    setRenaming(false);
    if (trimmed && trimmed !== project.name) await onRename(project.id, trimmed);
  }

  function onRenameKeyDown(e) {
    if (e.key === 'Enter') submitRename();
    if (e.key === 'Escape') { setRenaming(false); setRenameValue(project.name); }
  }

  function handleDelete() {
    setShowMenu(false);
    onDelete(project.id);
  }

  function handleClearSessions() {
    setShowMenu(false);
    onClearSessions(sessions);
  }

  function startSessionRename(session) {
    setSessionMenuId(null);
    setRenamingSessionId(session.id);
    setSessionRenameValue(session.title?.trim() || 'Chat');
  }

  async function submitSessionRename(session) {
    const trimmed = sessionRenameValue.trim();
    setRenamingSessionId(null);
    if (trimmed && trimmed !== session.title) await onRenameSession(session.id, trimmed);
  }

  function onSessionRenameKeyDown(e, session) {
    if (e.key === 'Enter') submitSessionRename(session);
    if (e.key === 'Escape') {
      setRenamingSessionId(null);
      setSessionRenameValue('');
    }
  }

  function handleSessionDelete(session) {
    setSessionMenuId(null);
    onDeleteSession(session.id);
  }

  function handleSessionPin(session) {
    setSessionMenuId(null);
    onPinSession(session.id, !session.pinned);
  }

  return (
    <div className="sidebar__project">
      <div className={`sidebar__project-header ${isActive ? 'sidebar__project-header--active' : ''}`}>
        <button
          type="button"
          className="sidebar__project-toggle"
          onClick={() => !renaming && setExpanded((e) => !e)}
          title={project.folder_path}
        >
          <span className="sidebar__project-chevron" aria-hidden>{expanded ? '▾' : '▸'}</span>
          {renaming ? (
            <input
              className="sidebar__project-rename-input"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onBlur={submitRename}
              onKeyDown={onRenameKeyDown}
              onClick={(e) => e.stopPropagation()}
              autoFocus
            />
          ) : (
            <span className="sidebar__project-name">{project.name}</span>
          )}
        </button>

        <div className="sidebar__project-actions" ref={menuRef}>
          <button
            type="button"
            className="sidebar__project-action-btn"
            onClick={(e) => { e.stopPropagation(); setShowMenu((v) => !v); }}
            title="Project options"
            aria-label="Project options"
          >
            ···
          </button>
          <button
            type="button"
            className="sidebar__project-new-chat"
            onClick={onNewChat}
            title="New Chat"
            aria-label={`New chat in ${project.name}`}
          >
            +
          </button>

          {showMenu && (
            <div className="context-menu" role="menu">
              <button type="button" className="context-menu__item" role="menuitem" onClick={startRename}>
                Rename
              </button>
              <button type="button" className="context-menu__item" role="menuitem" onClick={handleClearSessions}>
                Clear sessions
              </button>
              <button type="button" className="context-menu__item context-menu__item--danger" role="menuitem" onClick={handleDelete}>
                Remove
              </button>
            </div>
          )}
        </div>
      </div>

      {expanded && (
        <div className="sidebar__project-body">
          {(showAllSessions ? sessions : sessions.slice(0, SESSION_PAGE_SIZE)).map((s) => (
            <div key={s.id} className="sidebar__session-row">
              {renamingSessionId === s.id ? (
                <input
                  className="sidebar__session-rename-input"
                  value={sessionRenameValue}
                  onChange={(e) => setSessionRenameValue(e.target.value)}
                  onBlur={() => submitSessionRename(s)}
                  onKeyDown={(e) => onSessionRenameKeyDown(e, s)}
                  autoFocus
                />
              ) : (
                <button
                  type="button"
                  className={`sidebar__session-item ${s.id === selectedSessionId ? 'sidebar__session-item--active' : ''}`}
                  onClick={() => onSelectSession(s.id)}
                  title={s.title || 'Chat'}
                >
                  <span className="sidebar__session-title">{s.pinned ? '★ ' : ''}{s.title?.trim() || 'Chat'}</span>
                  <span className="sidebar__session-meta">{formatSessionMeta(s.created_at)}</span>
                </button>
              )}
              <div className="sidebar__session-actions" ref={sessionMenuId === s.id ? sessionMenuRef : null}>
                <button
                  type="button"
                  className="sidebar__session-action-btn"
                  onClick={(e) => { e.stopPropagation(); setSessionMenuId((id) => id === s.id ? null : s.id); }}
                  title="Session options"
                  aria-label="Session options"
                >
                  ···
                </button>
                {sessionMenuId === s.id && (
                  <div className="context-menu context-menu--session" role="menu">
                    <button type="button" className="context-menu__item" role="menuitem" onClick={() => handleSessionPin(s)}>
                      {s.pinned ? 'Unpin' : 'Pin'}
                    </button>
                    <button type="button" className="context-menu__item" role="menuitem" onClick={() => startSessionRename(s)}>
                      Rename
                    </button>
                    <button type="button" className="context-menu__item context-menu__item--danger" role="menuitem" onClick={() => handleSessionDelete(s)}>
                      Delete
                    </button>
                  </div>
                )}
              </div>
            </div>
          ))}
          {!showAllSessions && sessions.length > SESSION_PAGE_SIZE && (
            <button
              type="button"
              className="sidebar__show-more"
              onClick={() => setShowAllSessions(true)}
            >
              Show {sessions.length - SESSION_PAGE_SIZE} more…
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// --- CreateProjectModal ---

function CreateProjectModal({ app, onCreate, onClose }) {
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

function InfoModal({ title, onClose, children }) {
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

function InfoList({ items, emptyText }) {
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

function MCPModal({ projectID, app, onClose }) {
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

function AgentsModal({ projectID, app, onClose }) {
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

function DiffDrawer({ projectID, app, onClose }) {
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

function SkillsPopup({ skills, filter, selected, onSelect, onHighlight }) {
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

function ChatInput({ onSend, onCancel, loading, error, onDismissError, currentProject, app, approvalRequest, onRespond, toolActivity, runStatus, sessionId, onRunStatusContext }) {
  const [prompt, setPrompt] = useState('');

  // Skills popup state
  const [skills, setSkills] = useState([]);
  const [skillsSelected, setSkillsSelected] = useState(0);

  // Status bar state (loaded per project)
  const [models, setModels] = useState([]);
  const [currentModel, setCurrentModel] = useState('');
  const [gitBranch, setGitBranch] = useState('');
  const [showModelDropdown, setShowModelDropdown] = useState(false);
  const modelDropdownRef = useRef(null);

  // Lazy-loaded panel state
  const [showMCP, setShowMCP] = useState(false);
  const [showAgents, setShowAgents] = useState(false);
  const [showDiff, setShowDiff] = useState(false);

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
                  {m.managed && (
                    <span className="model-selector__option-sub">via {m.destination}</span>
                  )}
                  {m.is_current && <span className="model-selector__option-check" aria-hidden>✓</span>}
                </button>
              ))}
            </div>
          )}
        </div>

        <span className="chat-status-bar__run" title={formatRunStatus(runStatus)}>
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
      </div>

      {showMCP && (
        <MCPModal projectID={currentProject.id} app={app} onClose={() => setShowMCP(false)} />
      )}
      {showAgents && (
        <AgentsModal projectID={currentProject.id} app={app} onClose={() => setShowAgents(false)} />
      )}
      {showDiff && (
        <DiffDrawer projectID={currentProject.id} app={app} onClose={() => setShowDiff(false)} />
      )}
    </div>
  );
}
