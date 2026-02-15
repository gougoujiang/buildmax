/**
 * API base URL and login. VITE_API_BASE defaults to http://localhost:5678.
 */
const defaultApiBase = "http://localhost:5678"

export function getApiBase(): string {
  const base = import.meta.env.VITE_API_BASE
  return typeof base === "string" && base !== "" ? base : defaultApiBase
}

export interface LoginUser {
  id: string
  email: string
  name: string
}

export interface LoginResponse {
  token: string
  user: LoginUser
}

export async function login(email: string): Promise<LoginResponse> {
  const res = await fetch(`${getApiBase()}/api/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const text = await res.text()
    let msg: string
    try {
      const j = JSON.parse(text) as { error?: string }
      msg = j.error ?? text
    } catch {
      msg = text || res.statusText
    }
    throw new Error(msg)
  }
  return res.json() as Promise<LoginResponse>
}

/** Workspace as returned by GET /api/workspaces (snake_case). */
export interface ApiWorkspace {
  id: string
  name: string
  owner_user_id?: string
  created_at?: number
}

export async function getWorkspaces(token: string): Promise<ApiWorkspace[]> {
  const res = await fetch(`${getApiBase()}/api/workspaces`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    const text = await res.text()
    let msg: string
    try {
      const j = JSON.parse(text) as { error?: string }
      msg = j.error ?? text
    } catch {
      msg = text || res.statusText
    }
    throw new Error(msg)
  }
  return res.json() as Promise<ApiWorkspace[]>
}
