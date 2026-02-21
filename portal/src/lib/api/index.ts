/**
 * Portal API layer: transport, DTOs, mappers, and fetch functions.
 * Import from "../lib/api" (or "./api") for all API access.
 */

import type { ExploreNode } from "../types"
import { getApiBase, checkUnauthorized, throwIfNotOk, parseErrorResponse, UNAUTHORIZED_EVENT } from "./client"
import type {
  ApiAgent,
  ApiArtifact,
  ApiArtifactItem,
  ApiProject,
  ApiSession,
  ApiTask,
  ApiTasksListResponse,
  ApiWorkspace,
  CreateTaskRunResponse,
  LoginResponse,
  LoginUser,
  OtpRequestResponse,
  UploadResponse,
} from "./types"
export { UNAUTHORIZED_EVENT, parseErrorResponse, getApiBase }
export type { LoginUser, LoginResponse }
export type {
  ApiWorkspace,
  ApiProject,
  ApiAgent,
  ApiTask,
  ApiTasksListResponse,
  ApiSession,
  ApiSessionMessage,
  CreateTaskRunResponse,
  ApiArtifact,
  ApiArtifactItem,
  UploadResponse,
} from "./types"
export {
  formatRelativeTime,
  apiAgentToAgent,
  apiArtifactToArtifact,
  apiTaskToTask,
  apiProjectToProject,
} from "./mappers"

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

export interface GetTasksPaginatedOptions {
  limit?: number
  offset?: number
  executedOnly?: boolean
}

export async function getTasksPaginated(
  workspaceId: string,
  token: string,
  options?: GetTasksPaginatedOptions
): Promise<ApiTasksListResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  if (options?.executedOnly) params.set("executed_only", "true")
  const q = params.toString()
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks${q ? `?${q}` : ""}`
  const res = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiTasksListResponse>
}

export async function getAgents(
  workspaceId: string,
  token: string
): Promise<ApiAgent[]> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
    { headers: { Authorization: `Bearer ${token}` } }
  )
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ApiAgent[]>
}

export async function createAgent(
  workspaceId: string,
  body: { name: string; description?: string; instructions?: string },
  token: string
): Promise<ApiAgent> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
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
  await throwIfNotOk(res)
  return res.json() as Promise<ApiAgent>
}

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
    const msg = await parseErrorResponse(res, "A run is already in progress for this task")
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<CreateTaskRunResponse>
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

export async function uploadFiles(
  workspaceId: string,
  files: File[],
  token: string,
  paths?: string[]
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
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/upload`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: formData,
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<UploadResponse>
}

export async function getFileTree(workspaceId: string, token: string): Promise<ExploreNode> {
  const res = await fetch(`${getApiBase()}/api/workspaces/${workspaceId}/files`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  checkUnauthorized(res)
  await throwIfNotOk(res)
  return res.json() as Promise<ExploreNode>
}

export async function getFileContent(
  workspaceId: string,
  filePath: string,
  token: string
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
