// Turning values into the strings the interface shows. Nothing here touches
// React or the Go side, which is why it can be read and changed on its own.

export function formatSessionMeta(createdAt) {
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

export function compareRecent(a, b) {
  return String(b || '').localeCompare(String(a || ''));
}

export function folderBaseName(path) {
  if (!path) return '';
  return path.split(/[/\\]/).filter(Boolean).pop() ?? path;
}

export function formatToolArgs(raw) {
  if (!raw) return '';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export function toolDisplayName(name) {
  const first = String(name || '').split('_', 1)[0];
  if (!first) return String(name || '');
  return first.slice(0, 1).toUpperCase() + first.slice(1);
}

export function shortToolArgs(raw) {
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

export function formatTokenCount(n) {
  const value = Number(n) || 0;
  if (value < 1000) return String(value);
  if (value % 1000 === 0) return `${value / 1000}k`;
  return `${(value / 1000).toFixed(1)}k`;
}

export function formatRunStatus(status) {
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

export function statusGlyph(status) {
  switch (status) {
    case 'added': return '+';
    case 'deleted': return '-';
    case 'renamed': return '↔';
    default: return '●';
  }
}

export function statusTitle(status) {
  switch (status) {
    case 'added': return 'Added';
    case 'deleted': return 'Deleted';
    case 'renamed': return 'Renamed';
    default: return 'Modified';
  }
}

export function displayDiffPath(file) {
  if (file?.status === 'renamed' && file.old_path) return `${file.old_path} → ${file.path}`;
  return file?.path ?? '';
}

export function splitPathForDisplay(path) {
  const value = String(path ?? '');
  const parts = value.split(/[/\\]/);
  const name = parts.pop() || value;
  const dir = parts.length ? `${parts.join('/')}/` : '';
  return { dir, name };
}

export function truncateMiddleText(value, max = 34) {
  const text = String(value ?? '');
  if (text.length <= max) return text;
  if (max <= 1) return '…';
  const head = Math.floor((max - 1) / 2);
  const tail = max - 1 - head;
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

export function parseRangeStart(token) {
  const n = Number.parseInt(token.replace(/^[-+]/, '').split(',')[0], 10);
  return Number.isFinite(n) ? n : 0;
}

export function parsePatchLines(patch) {
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
