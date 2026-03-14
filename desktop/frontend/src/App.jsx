import { useState, useRef, useEffect } from 'react';
import { ThemeProvider } from './ThemeContext';
import { ThemeToggle } from './ThemeToggle';

const MOCK_THREADS = [
  { id: '1', title: 'Summarize project README', updatedAt: '2 min ago' },
  { id: '2', title: 'Add unit tests for config', updatedAt: '1 hour ago' },
  { id: '3', title: 'Refactor agent loop', updatedAt: 'Yesterday' },
  { id: '4', title: 'Setup Wails desktop app', updatedAt: 'Mar 12' },
];

const MOCK_MESSAGES = {
  '1': [
    { role: 'user', content: 'Can you summarize the project README in three bullet points?' },
    { role: 'assistant', content: '1. **BuildMax** is an AI Agent project for building a general-purpose agent.\n2. It supports **CLI/TUI** (Bubble Tea) and a **web Portal** (React + Vite).\n3. Core is **Go** with LLM integration, tool calling, and optional workspace AGENTS.md.' },
  ],
  '2': [
    { role: 'user', content: 'Add unit tests for the config package.' },
    { role: 'assistant', content: "I'll add tests for DataDir, LogsDir, and LogLevel with BUILDMAX_HOME and BUILDMAX_LOG_LEVEL set." },
  ],
  '3': [
    { role: 'user', content: 'How should we refactor the agent loop?' },
    { role: 'assistant', content: 'Consider extracting tool execution into a separate step and adding a max-iterations guard.' },
  ],
  '4': [
    { role: 'user', content: 'Setup the basics of the Wails desktop app.' },
    { role: 'assistant', content: 'Done. Added cmd/buildmax-desktop, internal/cmd/desktop, minimal window, and make run desktop.' },
  ],
};

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

function MessageContent({ text }) {
  const html = escapeHtml(text).replace(/\n/g, '<br>');
  return <div className="message-content" dangerouslySetInnerHTML={{ __html: html }} />;
}

export default function App() {
  const [selectedId, setSelectedId] = useState(MOCK_THREADS[0]?.id ?? null);
  const historyRef = useRef(null);

  const thread = MOCK_THREADS.find((t) => t.id === selectedId);
  const messages = selectedId ? MOCK_MESSAGES[selectedId] : null;

  useEffect(() => {
    if (historyRef.current) historyRef.current.scrollTop = historyRef.current.scrollHeight;
  }, [selectedId]);

  return (
    <ThemeProvider>
      <div className="app">
        <aside className="sidebar">
          <div className="sidebar-header">
            <h2 className="sidebar-title">Chats</h2>
          </div>
          <ul className="thread-list" role="list">
            {MOCK_THREADS.map((t) => (
              <li
                key={t.id}
                className={`thread-item ${t.id === selectedId ? 'active' : ''}`}
                role="button"
                tabIndex={0}
                onClick={() => setSelectedId(t.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setSelectedId(t.id);
                  }
                }}
              >
                <span className="thread-title">{t.title}</span>
                <span className="thread-meta">{t.updatedAt}</span>
              </li>
            ))}
          </ul>
        </aside>
        <main className="main-panel">
          <div className="chat-header">
            <span className="chat-title">{thread ? thread.title : 'Select a chat'}</span>
            <ThemeToggle />
          </div>
        <div className="chat-history" ref={historyRef} role="log" aria-live="polite">
          {!messages && <p className="chat-empty">Select a chat from the list.</p>}
          {messages?.map((m, i) => (
            <div key={i} className={`message message--${m.role}`} role="listitem">
              <div className="message-role">{m.role === 'user' ? 'You' : 'Assistant'}</div>
              <MessageContent text={m.content} />
            </div>
          ))}
        </div>
        </main>
      </div>
    </ThemeProvider>
  );
}
