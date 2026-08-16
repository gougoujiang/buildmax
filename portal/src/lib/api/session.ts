/**
 * Stored session: the two credentials a login returns, and nothing else.
 *
 * This module deliberately makes no network calls. The transport in client.ts
 * needs to read the current token while deciding whether to retry a request,
 * and if the two imported each other that cycle would have to be untangled at
 * the worst possible moment. State lives here; the refresh that changes it
 * lives there.
 *
 * Every read goes to localStorage rather than to a cached copy, so two tabs
 * that refresh independently see each other's result instead of one of them
 * holding a token the other has already replaced.
 */

const ACCESS_TOKEN_KEY = "buildmax_token"
const REFRESH_TOKEN_KEY = "buildmax_refresh_token"
const EXPIRES_AT_KEY = "buildmax_token_expires_at"
const USER_KEY = "buildmax_user"

export interface StoredSession {
  accessToken: string
  /** Absent for a session stored before refresh tokens existed. */
  refreshToken: string | null
  /** Unix milliseconds. Absent when the server did not say. */
  expiresAt: number | null
}

export function currentAccessToken(): string | null {
  try {
    return localStorage.getItem(ACCESS_TOKEN_KEY)
  } catch {
    return null
  }
}

export function currentRefreshToken(): string | null {
  try {
    return localStorage.getItem(REFRESH_TOKEN_KEY)
  } catch {
    return null
  }
}

export function accessTokenExpiresAt(): number | null {
  try {
    const raw = localStorage.getItem(EXPIRES_AT_KEY)
    if (!raw) return null
    const parsed = Number(raw)
    return Number.isFinite(parsed) ? parsed : null
  } catch {
    return null
  }
}

export function writeSession(session: StoredSession): void {
  try {
    localStorage.setItem(ACCESS_TOKEN_KEY, session.accessToken)
    if (session.refreshToken) {
      localStorage.setItem(REFRESH_TOKEN_KEY, session.refreshToken)
    } else {
      localStorage.removeItem(REFRESH_TOKEN_KEY)
    }
    if (session.expiresAt) {
      localStorage.setItem(EXPIRES_AT_KEY, String(session.expiresAt))
    } else {
      localStorage.removeItem(EXPIRES_AT_KEY)
    }
  } catch {
    // A browser with storage disabled still works for one page load.
  }
}

export function clearSession(): void {
  try {
    localStorage.removeItem(ACCESS_TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
    localStorage.removeItem(EXPIRES_AT_KEY)
    localStorage.removeItem(USER_KEY)
  } catch {
    // Nothing to clear if storage was never readable.
  }
}

/** expiresIn is the server's seconds-from-now; absent means unknown. */
export function expiresAtFrom(expiresIn: number | undefined): number | null {
  if (typeof expiresIn !== "number" || !Number.isFinite(expiresIn) || expiresIn <= 0) {
    return null
  }
  return Date.now() + expiresIn * 1000
}

export function readStoredUser<T>(): T | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    return null
  }
}

export function writeStoredUser(user: unknown): void {
  try {
    localStorage.setItem(USER_KEY, JSON.stringify(user))
  } catch {
    // See writeSession.
  }
}
