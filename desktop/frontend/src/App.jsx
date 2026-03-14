import { useState, useRef, useEffect } from 'react';
import { ThemeProvider } from './ThemeContext';
import { ThemeToggle } from './ThemeToggle';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

function getApp() {
  if (typeof window === 'undefined') return null;
  const go = window.go;
  if (!go) return null;
  // Wails generates bindings at window.go.<package>.<Struct>; App is in package desktop
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
    if (diffMins < 60) return `${diffMins} min ago`;
    if (diffHours < 24) return `${diffHours} hour ago`;
    if (diffDays < 7) return `${diffDays} day ago`;
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  } catch {
    return '';
  }
}

export default function App() {
  const [sessions, setSessions] = useState([]);
  const [selectedId, setSelectedId] = useState(null);
  const [messages, setMessages] = useState([]);
  const [sessionTitle, setSessionTitle] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [recentCollapsed, setRecentCollapsed] = useState(false);
  const [recentVisibleCount, setRecentVisibleCount] = useState(10);
  const [wailsReady, setWailsReady] = useState(false);

  const RECENT_PAGE_SIZE = 10;
  const sortedSessions = [...sessions].sort((a, b) =>
    (b.created_at || '').localeCompare(a.created_at || '')
  );
  const visibleSessions = sortedSessions.slice(0, recentVisibleCount);
  const hasMoreRecent = sortedSessions.length > recentVisibleCount;
  const historyRef = useRef(null);
  const streamingContentRef = useRef('');

  // Stream event names (must match Go desktop package)
  const EV_STREAM_DELTA = 'desktop/stream-delta';
  const EV_STREAM_DONE = 'desktop/stream-done';
  const EV_STREAM_ERROR = 'desktop/stream-error';

  // Subscribe to stream events (one stream at a time; loading prevents overlapping)
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
      setMessages((prev) => {
        const next = [...prev];
        const last = next[next.length - 1];
        if (last?.role === 'assistant') {
          next[next.length - 1] = { ...last, content: reply };
        }
        return next;
      });
      setSelectedId(sessionId || null);
      if (sessionId) {
        setSessionTitle('Chat');
        getApp()?.GetSession(sessionId).then((detail) => {
          setSessionTitle(detail?.title?.trim() || 'Chat');
        }).catch(() => {});
      }
      getApp()?.ListSessions().then((list) => setSessions(list ?? [])).catch(() => {});
      setLoading(false);
    });
    const unsubError = EventsOn(EV_STREAM_ERROR, (payload) => {
      const message = payload?.message ?? 'Stream error';
      streamingContentRef.current = '';
      setError(message);
      setMessages((prev) => {
        const last = prev[prev.length - 1];
        if (last?.role === 'assistant' && last?.content === '') {
          return prev.slice(0, -1);
        }
        return prev;
      });
      setLoading(false);
    });
    return () => {
      EventsOff(EV_STREAM_DELTA, EV_STREAM_DONE, EV_STREAM_ERROR);
      unsubDelta?.();
      unsubDone?.();
      unsubError?.();
    };
  }, []);

  // Wails injects window.go after the page loads; wait a tick before deciding backend is missing
  useEffect(() => {
    if (getApp()) {
      setWailsReady(true);
      return;
    }
    const id = setTimeout(() => setWailsReady(true), 150);
    return () => clearTimeout(id);
  }, []);

  const app = getApp();

  function refreshSessions() {
    if (!app) return;
    app.ListSessions()
      .then((list) => setSessions(list ?? []))
      .catch((err) => setError(err?.message ?? String(err)));
  }

  useEffect(() => {
    refreshSessions();
  }, []);

  useEffect(() => {
    if (selectedId === null) {
      setMessages([]);
      setSessionTitle('New Chat');
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
  }, [selectedId, app]);

  useEffect(() => {
    if (historyRef.current) {
      historyRef.current.scrollTop = historyRef.current.scrollHeight;
    }
  }, [messages]);

  async function handleSend(prompt) {
    if (!prompt?.trim() || loading || !app) return;
    setLoading(true);
    setError(null);
    streamingContentRef.current = '';
    const sessionIdToUse = selectedId || '';
    const trimmed = prompt.trim();
    setMessages((prev) => [
      ...prev,
      { role: 'user', content: trimmed },
      { role: 'assistant', content: '' },
    ]);
    try {
      await app.SendMessageStream(sessionIdToUse, trimmed);
      // Result and list updates happen via desktop/stream-done or desktop/stream-error
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

  if (!wailsReady) {
    return (
      <ThemeProvider>
        <div className="shell">
          <div className="shell__body" style={{ padding: '2rem' }}>
            <p className="page-chat__text page-chat__muted">Loading…</p>
          </div>
        </div>
      </ThemeProvider>
    );
  }
  if (!app) {
    return (
      <ThemeProvider>
        <div className="shell">
          <div className="shell__body" style={{ padding: '2rem' }}>
            <p className="page-chat__text page-chat__muted">
              Run this app with Wails to use chat (e.g. <code>wails dev</code> or <code>./make run desktop</code>).
              If you already did, the backend may not be bound yet—try refreshing.
            </p>
          </div>
        </div>
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider>
      <div className="shell">
        <div className="shell__body">
          <aside className="sidebar" aria-label="Sidebar">
            <div className="sidebar__header">
              <h1 className="sidebar__logo-title">BuildMax</h1>
            </div>
            <nav className="sidebar__nav" aria-label="Primary">
              <div className="sidebar__section">
                <button
                  type="button"
                  className={`sidebar__nav-item ${selectedId === null ? 'sidebar__nav-item--active' : ''}`}
                  onClick={() => setSelectedId(null)}
                >
                  <span className="sidebar__nav-icon" aria-hidden>+</span>
                  <span className="sidebar__nav-item-text">New Chat</span>
                </button>
                <div className="sidebar__chats">
                  <button
                    type="button"
                    className="sidebar__chats-toggle"
                    onClick={() => setRecentCollapsed((c) => !c)}
                    aria-expanded={!recentCollapsed}
                    aria-controls="sidebar-conversations-list"
                  >
                    <span className="sidebar__nav-icon" aria-hidden>⊙</span>
                    <span className="sidebar__heading">Recent</span>
                    <span className="sidebar__chats-chevron" aria-hidden>
                      {recentCollapsed ? '▶' : '▼'}
                    </span>
                  </button>
                  <div
                    id="sidebar-conversations-list"
                    className="sidebar__chats-list"
                    hidden={recentCollapsed}
                  >
                    <ul className="sidebar__list">
                      {visibleSessions.map((s) => (
                        <li key={s.id} className="sidebar__item">
                          <button
                            type="button"
                            className={`sidebar__link ${selectedId === s.id ? 'sidebar__link--active' : ''}`}
                            onClick={() => setSelectedId(s.id)}
                          >
                            <span className="sidebar__chat-title">
                              {s.title?.trim() || 'Chat'}
                            </span>
                            <span className="sidebar__chat-meta">
                              {formatSessionMeta(s.created_at)}
                            </span>
                          </button>
                        </li>
                      ))}
                    </ul>
                    {hasMoreRecent && (
                      <div className="sidebar__see-more">
                        <button
                          type="button"
                          className="sidebar__see-more-btn"
                          onClick={() => setRecentVisibleCount((n) => n + RECENT_PAGE_SIZE)}
                        >
                          See More
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </nav>
          </aside>
          <main className="shell__main">
            <div className="shell__top">
              <span className="shell__title" style={{ flex: 1, minWidth: 0, fontWeight: 600 }}>
                {sessionTitle}
              </span>
              <ThemeToggle />
            </div>
            <div className="shell__content">
              <div className="page-chat">
                <section
                  ref={historyRef}
                  className="page-chat__history"
                  aria-label="Conversation history"
                  role="log"
                  aria-live="polite"
                >
                  {messages.length === 0 && selectedId !== null && (
                    <p className="page-chat__text page-chat__muted">No messages yet.</p>
                  )}
                  {messages.length === 0 && selectedId === null && (
                    <p className="page-chat__text page-chat__muted">
                      Type a message below to start a new chat.
                    </p>
                  )}
                  {messages.map((m, i) => (
                    <div
                      key={i}
                      className={`page-chat__msg-row page-chat__msg-row--${m.role}`}
                      role="article"
                      aria-label={m.role === 'user' ? 'You' : m.role}
                    >
                      <span className="page-chat__msg-icon" aria-hidden>
                        {m.role === 'user' ? 'U' : 'A'}
                      </span>
                      <div className={`page-chat__msg page-chat__msg--${m.role}`}>
                        <div className="page-chat__msg-content">{m.content}</div>
                      </div>
                    </div>
                  ))}
                </section>
                <section className="page-chat__input" aria-label="Send a message">
                  <ChatInput
                    onSend={handleSend}
                    loading={loading}
                    error={error}
                    onDismissError={() => setError(null)}
                  />
                </section>
              </div>
            </div>
          </main>
        </div>
      </div>
    </ThemeProvider>
  );
}

function ChatInput({ onSend, loading, error, onDismissError }) {
  const [prompt, setPrompt] = useState('');

  function handleSubmit() {
    const value = prompt.trim();
    if (!value || loading) return;
    onSend(value);
    setPrompt('');
  }

  return (
    <>
      <div className="page-chat__input-box">
        <textarea
          className="page-chat__follow-up-input"
          value={prompt}
          onChange={(e) => {
            setPrompt(e.target.value);
            onDismissError();
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              handleSubmit();
            }
          }}
          placeholder="Type a message… (Enter to send, Shift+Enter for new line)"
          rows={2}
          disabled={loading}
          aria-label="Message"
        />
        <button
          type="button"
          className="page-chat__follow-up-btn"
          onClick={handleSubmit}
          disabled={loading || !prompt.trim()}
        >
          {loading ? 'Sending…' : 'Send'}
        </button>
      </div>
      {error && (
        <p className="page-chat__text page-chat__error" role="alert">
          {error}
        </p>
      )}
    </>
  );
}
