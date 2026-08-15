/**
 * API transport: base URL, 401 handling, and response helpers.
 * No DTOs or UI types.
 */

const defaultApiBase = "http://localhost:5678"

declare global {
  interface Window {
    /**
     * Runtime configuration. `public/config.js` ships an empty default and the
     * container entrypoint overwrites it from BUILDMAX_API_BASE before nginx
     * starts, which is what lets one published image serve every deployment:
     * the API URL is not knowable when the bundle is built.
     */
    __BUILDMAX_CONFIG__?: { apiBase?: string }
  }
}

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
 * API base URL, most specific source first:
 *
 *  1. `window.__BUILDMAX_CONFIG__.apiBase` — written at container start from
 *     BUILDMAX_API_BASE. The only source available to a prebuilt image.
 *  2. `VITE_API_BASE` — baked in at build time, for `npm run dev` and for
 *     anyone building the bundle themselves.
 *  3. `http://localhost:5678` — the default a local server listens on.
 *
 * A trailing slash is trimmed, so `BUILDMAX_API_BASE=/` yields "" and every
 * request goes to the origin serving the Portal — the setup to use when a
 * reverse proxy puts the Portal and the server behind one hostname.
 */
export function getApiBase(): string {
  const runtime = typeof window !== "undefined" ? window.__BUILDMAX_CONFIG__?.apiBase : undefined
  if (typeof runtime === "string" && runtime !== "") {
    return trimTrailingSlash(runtime)
  }
  const build = import.meta.env.VITE_API_BASE
  if (typeof build === "string" && build !== "") {
    return trimTrailingSlash(build)
  }
  return defaultApiBase
}

function trimTrailingSlash(url: string): string {
  return url.replace(/\/+$/, "")
}
