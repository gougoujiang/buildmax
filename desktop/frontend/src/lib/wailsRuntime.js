function getRuntime() {
  if (typeof window === 'undefined') return null;
  return window.runtime ?? null;
}

export function EventsOn(eventName, callback) {
  const runtime = getRuntime();
  if (!runtime?.EventsOn) {
    return () => {};
  }
  return runtime.EventsOn(eventName, callback);
}

export function EventsOff(...eventNames) {
  const runtime = getRuntime();
  if (!runtime?.EventsOff) return;
  runtime.EventsOff(...eventNames);
}
