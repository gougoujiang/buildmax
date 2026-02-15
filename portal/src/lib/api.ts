import type { Project } from "./types"

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

/** Project as returned by GET/POST /api/workspaces/{id}/projects (snake_case). */
export interface ApiProject {
  id: string
  workspace_id: string
  name: string
  description: string
  created_at: number
}

export async function getProjects(workspaceId: string, token: string): Promise<ApiProject[]> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/projects`, {
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
  return res.json() as Promise<ApiProject[]>
}

export async function createProject(
  workspaceId: string,
  body: { name: string; description?: string },
  token: string
): Promise<ApiProject> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/projects`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
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
  return res.json() as Promise<ApiProject>
}

/** Map API project to UI Project (status/updatedAtLabel derived from created_at). */
export function apiProjectToProject(api: ApiProject): Project {
  const created = new Date(api.created_at * 1000)
  const label =
    created.toDateString() === new Date().toDateString()
      ? "Created today"
      : `Created ${created.toLocaleDateString()}`
  return {
    id: api.id,
    workspaceId: api.workspace_id,
    name: api.name,
    status: "active",
    updatedAtLabel: label,
  }
}
