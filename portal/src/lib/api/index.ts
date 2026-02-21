/**
 * Portal API layer: transport, DTOs, mappers, and fetch functions.
 * Import from "../lib/api" (or "./api") for all API access.
 */

import type { ExploreNode } from "../types"
import { getApiBase, checkUnauthorized, parseErrorResponse, requestJson, requestText, throwIfNotOk, UNAUTHORIZED_EVENT } from "./client"
import type {
  ApiAgent,
  ApiArtifact,
  ApiArtifactItem,
  ApiChat,
  ApiChatsListResponse,
  ApiSession,
  ApiWorkspace,
  CreateChatRunResponse,
  LoginResponse,
  LoginUser,
  OtpRequestResponse,
  UploadResponse,
} from "./types"
export { UNAUTHORIZED_EVENT, parseErrorResponse, getApiBase }
export type { LoginUser, LoginResponse }
export type {
  ApiWorkspace,
  ApiAgent,
  ApiChat,
  ApiChatsListResponse,
  ApiSession,
  ApiSessionMessage,
  CreateChatRunResponse,
  ApiArtifact,
  ApiArtifactItem,
  UploadResponse,
} from "./types"
export {
  apiAgentToAgent,
  apiArtifactToArtifact,
  apiChatToChat,
} from "./mappers"

const jsonHeaders = { "Content-Type": "application/json" }

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` }
}

export async function requestOtp(
  email: string,
  intent: "signup" | "login"
): Promise<OtpRequestResponse> {
  return requestJson<OtpRequestResponse>(`${getApiBase()}/api/otp/request`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ email, intent }),
  })
}

export async function login(email: string, otp: string): Promise<LoginResponse> {
  return requestJson<LoginResponse>(`${getApiBase()}/api/login`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ email, otp }),
  })
}

export async function getWorkspaces(token: string): Promise<ApiWorkspace[]> {
  return requestJson<ApiWorkspace[]>(`${getApiBase()}/api/workspaces`, {
    headers: authHeaders(token),
  })
}

export async function createWorkspace(
  body: { name: string },
  token: string
): Promise<ApiWorkspace> {
  return requestJson<ApiWorkspace>(`${getApiBase()}/api/workspaces`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function getChats(
  workspaceId: string,
  token: string,
  projectId?: string
): Promise<ApiChat[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/chats`
  if (projectId) {
    url += `?project_id=${encodeURIComponent(projectId)}`
  }
  return requestJson<ApiChat[]>(url, { headers: authHeaders(token) })
}

export interface GetChatsPaginatedOptions {
  limit?: number
  offset?: number
  executedOnly?: boolean
}

export async function getChatsPaginated(
  workspaceId: string,
  token: string,
  options?: GetChatsPaginatedOptions
): Promise<ApiChatsListResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  if (options?.executedOnly) params.set("executed_only", "true")
  const q = params.toString()
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/chats${q ? `?${q}` : ""}`
  return requestJson<ApiChatsListResponse>(url, { headers: authHeaders(token) })
}

export async function getAgents(
  workspaceId: string,
  token: string
): Promise<ApiAgent[]> {
  return requestJson<ApiAgent[]>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
    { headers: authHeaders(token) }
  )
}

export async function createAgent(
  workspaceId: string,
  body: { name: string; description?: string; instructions?: string },
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function getChatConversation(
  workspaceId: string,
  chatId: string,
  token: string
): Promise<ApiSession | null> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/chats/${encodeURIComponent(chatId)}/conversation`,
    { headers: authHeaders(token) }
  )
  checkUnauthorized(res)
  if (res.status === 404) return null
  await throwIfNotOk(res)
  return res.json() as Promise<ApiSession>
}

export async function createChat(
  workspaceId: string,
  body: { input: string; project_id?: string },
  token: string
): Promise<ApiChat> {
  return requestJson<ApiChat>(`${getApiBase()}/api/workspaces/${workspaceId}/chats`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function createChatRun(
  workspaceId: string,
  chatId: string,
  body: { input: string },
  token: string
): Promise<CreateChatRunResponse> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/chats/${encodeURIComponent(chatId)}/runs`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
  checkUnauthorized(res)
  if (res.status === 409) {
    const msg = await parseErrorResponse(res, "A run is already in progress for this chat")
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<CreateChatRunResponse>
}

export async function getArtifacts(
  workspaceId: string,
  token: string,
  options?: { projectId?: string; chatId?: string }
): Promise<ApiArtifact[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/artifacts`
  const params = new URLSearchParams()
  if (options?.projectId) params.set("project_id", options.projectId)
  if (options?.chatId) params.set("chat_id", options.chatId)
  const q = params.toString()
  if (q) url += `?${q}`
  return requestJson<ApiArtifact[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactItems(
  workspaceId: string,
  chatRunId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(chatRunId)}/items`
  return requestJson<ApiArtifactItem[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactContent(
  workspaceId: string,
  chatRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(chatRunId)}/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
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
  return requestJson<UploadResponse>(`${getApiBase()}/api/workspaces/${workspaceId}/upload`, {
    method: "POST",
    headers: authHeaders(token),
    body: formData,
  })
}

export async function getFileTree(workspaceId: string, token: string): Promise<ExploreNode> {
  return requestJson<ExploreNode>(`${getApiBase()}/api/workspaces/${workspaceId}/files`, {
    headers: authHeaders(token),
  })
}

export async function getFileContent(
  workspaceId: string,
  filePath: string,
  token: string
): Promise<string> {
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/")
  return requestText(
    `${getApiBase()}/api/workspaces/${workspaceId}/files/${encodedPath}`,
    { headers: authHeaders(token) }
  )
}
