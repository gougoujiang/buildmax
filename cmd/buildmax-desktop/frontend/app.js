// Mock data for layout development
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
    { role: 'assistant', content: 'I\'ll add tests for DataDir, LogsDir, and LogLevel with BUILDMAX_HOME and BUILDMAX_LOG_LEVEL set.' },
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

function renderThreadList() {
  const list = document.getElementById('thread-list');
  list.innerHTML = MOCK_THREADS.map(
    (t) =>
      `<li class="thread-item" data-thread-id="${t.id}" role="button" tabindex="0">
        <span class="thread-title">${escapeHtml(t.title)}</span>
        <span class="thread-meta">${escapeHtml(t.updatedAt)}</span>
      </li>`
  ).join('');

  list.querySelectorAll('.thread-item').forEach((el) => {
    el.addEventListener('click', () => selectThread(el.dataset.threadId));
    el.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        selectThread(el.dataset.threadId);
      }
    });
  });
}

function escapeHtml(s) {
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

function selectThread(threadId) {
  document.querySelectorAll('.thread-item').forEach((el) => {
    el.classList.toggle('active', el.dataset.threadId === threadId);
  });
  const thread = MOCK_THREADS.find((t) => t.id === threadId);
  document.getElementById('chat-header').querySelector('.chat-title').textContent = thread ? thread.title : 'Select a chat';
  renderChatHistory(threadId);
}

function renderChatHistory(threadId) {
  const container = document.getElementById('chat-history');
  const messages = MOCK_MESSAGES[threadId];
  if (!messages) {
    container.innerHTML = '<p class="chat-empty">Select a chat from the list.</p>';
    return;
  }
  container.innerHTML = messages
    .map(
      (m) =>
        `<div class="message message--${m.role}" role="listitem">
          <div class="message-role">${m.role === 'user' ? 'You' : 'Assistant'}</div>
          <div class="message-content">${formatMessageContent(m.content)}</div>
        </div>`
    )
    .join('');
  container.scrollTop = container.scrollHeight;
}

function formatMessageContent(text) {
  return escapeHtml(text).replace(/\n/g, '<br>');
}

function init() {
  renderThreadList();
  renderChatHistory(null);
  if (MOCK_THREADS.length > 0) {
    selectThread(MOCK_THREADS[0].id);
  }
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
