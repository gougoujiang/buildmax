import {
  checkUnauthorized,
  getApiBase,
  parseErrorResponse,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import { readSSEStream } from "../../lib/api/sse"
import type {
  ApiChat,
  ApiChatsListResponse,
  ApiSession,
  CreateTaskRunResponse,
} from "../../lib/api/types"

export interface GetChatsPaginatedOptions {
  limit?: number
  offset?: number
  executedOnly?: boolean
}

export interface CreateChatBody {
  input?: string
  agent_id?: string
}

export interface RunStreamCallbacks {
  onDelta: (text: string) => void
  onDone: () => void
  onError: (err: Error) => void
}

export async function getChats(
  workspaceId: string,
  token: string
): Promise<ApiChat[]> {
  return requestJson<ApiChat[]>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks`,
    { headers: authHeaders(token) }
  )
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
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks${q ? `?${q}` : ""}`
  return requestJson<ApiChatsListResponse>(url, { headers: authHeaders(token) })
}

export async function getChatConversation(
  workspaceId: string,
  chatId: string,
  token: string
): Promise<ApiSession | null> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(chatId)}/conversation`,
    { headers: authHeaders(token) }
  )
  checkUnauthorized(res)
  if (res.status === 404) return null
  await throwIfNotOk(res)
  return res.json() as Promise<ApiSession>
}

export async function createChat(
  workspaceId: string,
  body: CreateChatBody,
  token: string
): Promise<ApiChat> {
  return requestJson<ApiChat>(`${getApiBase()}/api/workspaces/${workspaceId}/tasks`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function createTaskRun(
  workspaceId: string,
  chatId: string,
  body: { input: string },
  token: string
): Promise<CreateTaskRunResponse> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(chatId)}/runs`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
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

export function subscribeChatStream(
  workspaceId: string,
  chatId: string,
  token: string,
  callbacks: RunStreamCallbacks
): () => void {
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(chatId)}/stream`
  const controller = new AbortController()

  fetch(url, { headers: authHeaders(token), signal: controller.signal })
    .then(async (res) => {
      checkUnauthorized(res)
      if (!res.ok) {
        const msg = await parseErrorResponse(res, "Stream failed")
        callbacks.onError(new Error(msg))
        return
      }

      await readSSEStream(res, {
        onData: (data) => {
          if (data === "done") {
            callbacks.onDone()
            return false
          }
          callbacks.onDelta(data)
        },
        onDone: callbacks.onDone,
        onError: (err) => {
          if (err.name === "AbortError") return
          callbacks.onError(err)
        },
      })
    })
    .catch((err) => {
      if ((err as Error).name === "AbortError") return
      callbacks.onError(err instanceof Error ? err : new Error(String(err)))
    })

  return () => controller.abort()
}
