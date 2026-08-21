// Folding live events into the message list the thread renders. Each takes the
// current list and returns the next one; none mutates what it was given.

export function buildToolResultMap(messages) {
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

export function appendAssistantForNextLLM(messages) {
  const last = messages[messages.length - 1];
  if (!last) return messages;
  if (last.role === 'assistant' && !last.content && !(last.tool_calls || []).length) {
    return messages;
  }
  if (last.role === 'user') return [...messages, { role: 'assistant', content: '' }];
  return [...messages, { role: 'assistant', content: '' }];
}

export function addLiveToolCall(messages, call) {
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

export function addLiveToolResult(messages, payload) {
  const id = payload?.tool_call_id;
  if (!id || messages.some((m) => m.role === 'tool' && m.tool_call_id === id)) {
    return messages;
  }
  const content = payload?.is_error
    ? `error: ${payload?.reason || 'tool call failed'}`
    : '';
  return [...messages, { role: 'tool', tool_call_id: id, content }];
}

export function mergeRunStatus(prev, payload) {
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
