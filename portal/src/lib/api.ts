import type { Artifact, ExploreNode, Project, Task } from "./types"

/**
 * API base URL and login.
 * - When the portal is served from buildmax.kind.local (deployed in kind), use http://buildmax-api.kind.local.
 * - Otherwise use VITE_API_BASE if set, or http://localhost:5678 for local dev.
 */
const defaultApiBase = "http://localhost:5678"
const kindApiBase = "http://buildmax-api.kind.local"

/** Event dispatched when any API call returns 401. Listeners should clear auth and show login. */
export const UNAUTHORIZED_EVENT = "buildmax:unauthorized"

function checkUnauthorized(res: Response): void {
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT))
  }
}

/** If res is not ok, read body, parse error message, and throw. Call after checkUnauthorized. */
async function throwIfNotOk(res: Response): Promise<void> {
  if (res.ok) return
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

export function getApiBase(): string {
  if (typeof window !== "undefined" && window.location?.host === "buildmax.kind.local") {
    return kindApiBase
  }
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

export interface OtpRequestResponse {
  message: string
}

export async function requestOtp(
  email: string,
  intent: "signup" | "login"
): Promise<OtpRequestResponse> {
  const res = await fetch(`${getApiBase()}/api/otp/request`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, intent }),
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<OtpRequestResponse>
}

export async function login(email: string, otp: string): Promise<LoginResponse> {
  const res = await fetch(`${getApiBase()}/api/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, otp }),
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
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
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiWorkspace[]>
}

export async function createWorkspace(
  body: { name: string },
  token: string
): Promise<ApiWorkspace> {
  const res = await fetch(`${getApiBase()}/api/workspaces`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiWorkspace>
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
  checkUnauthorized(res)
  await throwIfNotOk(res)
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
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiProject>
}

/** Task as returned by GET/POST /api/workspaces/{id}/tasks (snake_case). */
export interface ApiTask {
  id: string
  workspace_id: string
  project_id: string | null
  session_id: string | null
  status: string
  input: string
  output: string | null
  created_by: string
  created_at: number
  started_at: number | null
  ended_at: number | null
  error_message: string | null
}

export async function getTasks(
  workspaceId: string,
  token: string,
  projectId?: string
): Promise<ApiTask[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/tasks`
  if (projectId) {
    url += `?project_id=${encodeURIComponent(projectId)}`
  }
  const res = await fetch(url, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiTask[]>
}

/** Conversation as returned by GET /api/workspaces/{id}/tasks/{id}/conversation (agent conversation for a task). */
export interface ApiSession {
  id: string
  title: string
  created_at: string
  messages: ApiSessionMessage[]
}

export interface ApiSessionMessage {
  role: string
  content: string
  tool_call_id?: string
  tool_calls?: { id: string; name: string; arguments?: string }[]
}

/** Returns conversation or null if not found (e.g. task not run yet). Throws on other errors. */
export async function getTaskConversation(
  workspaceId: string,
  taskId: string,
  token: string
): Promise<ApiSession | null> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/conversation`,
    { headers: { Authorization: `Bearer ${token}` } }
  )
  checkUnauthorized(res)
  if (res.status === 404) return null
  await throwIfNotOk(res)
  return res.json() as Promise<ApiSession>
}

export async function createTask(
  workspaceId: string,
  body: { input: string; project_id?: string },
  token: string
): Promise<ApiTask> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/tasks`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiTask>
}

/** Response from POST /api/workspaces/{id}/tasks/{task_id}/runs (snake_case). */
export interface CreateTaskRunResponse {
  run_id: string
  task_id: string
}

/**
 * Create a follow-up run for a task. Body: { input }. Returns run_id and task_id.
 * Throws with message "a run is already in progress for this task" on 409.
 */
export async function createTaskRun(
  workspaceId: string,
  taskId: string,
  body: { input: string },
  token: string
): Promise<CreateTaskRunResponse> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}/runs`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    }
  )
  checkUnauthorized(res)
  if (res.status === 409) {
    const text = await res.text()
    let msg: string
    try {
      const j = JSON.parse(text) as { error?: string }
      msg = j.error ?? "A run is already in progress for this task"
    } catch {
      msg = "A run is already in progress for this task"
    }
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<CreateTaskRunResponse>
}

/** Artifact as returned by GET /api/workspaces/{id}/artifacts (snake_case). */
export interface ApiArtifact {
  artifact_id: string
  task_id: string
  workspace_id: string
  project_id: string | null
  created_at: number
  seq: number
  task_input_snippet: string
}

export async function getArtifacts(
  workspaceId: string,
  token: string,
  options?: { projectId?: string; taskId?: string }
): Promise<ApiArtifact[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/artifacts`
  const params = new URLSearchParams()
  if (options?.projectId) params.set("project_id", options.projectId)
  if (options?.taskId) params.set("task_id", options.taskId)
  const q = params.toString()
  if (q) url += `?${q}`
  const res = await fetch(url, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiArtifact[]>
}

/** Artifact item (file) as returned by GET /api/workspaces/{id}/artifacts/{id}/items (snake_case). */
export interface ApiArtifactItem {
  relative_path: string
}

export async function getArtifactItems(
  workspaceId: string,
  artifactId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(artifactId)}/items`
  const res = await fetch(url, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiArtifactItem[]>
}

export async function getArtifactContent(
  workspaceId: string,
  artifactId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(artifactId)}/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  const res = await fetch(url, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.text()
}

function artifactTimeLabel(createdAt: number): string {
  const d = new Date(createdAt * 1000)
  const today = new Date()
  if (d.toDateString() === today.toDateString()) {
    return `Today ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  if (d.toDateString() === yesterday.toDateString()) {
    return `Yesterday ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  return d.toLocaleString()
}

/** Map API artifact to UI Artifact. */
export function apiArtifactToArtifact(api: ApiArtifact): Artifact {
  return {
    id: api.artifact_id,
    taskId: api.task_id,
    projectId: api.project_id ?? undefined,
    workspaceId: api.workspace_id,
    timeLabel: artifactTimeLabel(api.created_at),
    title: api.task_input_snippet || `Artifact ${api.artifact_id}`,
  }
}

/** Upload response from POST /api/workspaces/{id}/upload. */
export interface UploadResponse {
  uploaded: string[]
}

export async function uploadFiles(
  workspaceId: string,
  files: File[],
  token: string,
  paths?: string[],
): Promise<UploadResponse> {
  const formData = new FormData()
  for (const file of files) {
    formData.append("files", file)
  }
  if (paths) {
    for (const p of paths) {
      formData.append("paths", p)
    }
  }
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${workspaceId}/upload`,
    {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
      body: formData,
    }
  )
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<UploadResponse>
}

/** Fetch the full directory tree for a workspace. */
export async function getFileTree(
  workspaceId: string,
  token: string,
): Promise<ExploreNode> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/files`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ExploreNode>
}

/** Fetch file content as plain text. */
export async function getFileContent(
  workspaceId: string,
  filePath: string,
  token: string,
): Promise<string> {
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/")
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${workspaceId}/files/${encodedPath}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    }
  )
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.text()
}

function taskStatusToUI(status: string): Task["status"] {
  switch (status) {
    case "SUCCEEDED":
      return "success"
    case "FAILED":
      return "failed"
    case "CANCELED":
      return "canceled"
    case "PENDING":
      return "pending"
    case "RUNNING":
    default:
      return "running"
  }
}

function taskTimeLabel(api: ApiTask): string {
  const ts = api.ended_at ?? api.created_at
  const d = new Date(ts * 1000)
  const today = new Date()
  if (d.toDateString() === today.toDateString()) {
    return `Today ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  if (d.toDateString() === yesterday.toDateString()) {
    return `Yesterday ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  return d.toLocaleString()
}

/** Map API task to UI Task. */
export function apiTaskToTask(api: ApiTask): Task {
  const title = api.input.length > 80 ? api.input.slice(0, 77) + "..." : api.input
  const summary = api.output ?? (api.input.length > 120 ? api.input.slice(0, 117) + "..." : api.input)
  return {
    id: api.id,
    projectId: api.project_id ?? undefined,
    sessionId: api.session_id ?? undefined,
    title,
    status: taskStatusToUI(api.status),
    timeLabel: taskTimeLabel(api),
    summary,
  }
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
