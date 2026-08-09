/**
 * API transport: base URL, 401 handling, and response helpers.
 * No DTOs or UI types.
 */

const defaultApiBase = "http://localhost:5678"
const kindApiBase = "http://buildmax-api.kind.local"

/** Event dispatched when any API call returns 401. Listeners should clear auth and show login. */
export const UNAUTHORIZED_EVENT = "buildmax:unauthorized"

export function checkUnauthorized(res: Response): void {
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT))
  }
}

/** Parse response body to an error message. Uses JSON .error if present, else body text or default. */
export async function parseErrorResponse(res: Response, defaultMessage: string): Promise<string> {
  const text = await res.text()
  try {
    const j = JSON.parse(text) as { error?: string }
    return j.error ?? (text || defaultMessage)
  } catch {
    return text || defaultMessage
  }
}

/** If res is not ok, read body, parse error message, and throw. Call after checkUnauthorized. */
export async function throwIfNotOk(res: Response): Promise<void> {
  if (res.ok) return
  const msg = await parseErrorResponse(res, res.statusText)
  throw new Error(msg)
}

/** Fetch URL, check 401, throw if not ok, return JSON. Use for standard API calls. */
export async function requestJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<T>
}

/** Fetch URL, check 401, throw if not ok, return text. */
export async function requestText(url: string, init?: RequestInit): Promise<string> {
  const res = await fetch(url, init)
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.text()
}

/**
 * API base URL.
 * - When the portal is served from buildmax.kind.local (deployed in kind), use http://buildmax-api.kind.local.
 * - Otherwise use VITE_API_BASE if set, or http://localhost:5678 for local dev.
 */
export function getApiBase(): string {
  if (typeof window !== "undefined" && window.location?.host === "buildmax.kind.local") {
    return kindApiBase
  }
  const base = import.meta.env.VITE_API_BASE
  return typeof base === "string" && base !== "" ? base : defaultApiBase
}
