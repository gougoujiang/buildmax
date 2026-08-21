// Reaching the Go side. Wails injects the bindings on window at runtime, so
// this is a lookup rather than an import.
export function getApp() {
  if (typeof window === 'undefined') return null;
  const go = window.go;
  if (!go) return null;
  return go.desktop?.App ?? go.main?.App ?? go.App ?? null;
}
