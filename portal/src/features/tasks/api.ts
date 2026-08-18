import {
  apiFetch,
  getApiBase,
  parseErrorResponse,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import { readSSEStream } from "../../lib/api/sse"
import type {
  ApiTask,
  ApiTasksListResponse,
  ApiSession,
  CancelTaskResponse,
  CreateTaskRunResponse,
} from "../../lib/api/types"
import { createConversation } from "../conversations"

export interface GetTasksPaginatedOptions {
  limit?: number
  offset?: number
  executedOnly?: boolean
}

export interface CreateTaskBody {
  input?: string
  agent_id?: string
}

export interface RunStreamCallbacks {
  onDelta: (text: string) => void
  onDone: () => void
  onError: (err: Error) => void
}

export async function getTasks(
  teamId: string,
  conversationId: string,
  token: string
): Promise<ApiTask[]> {
  return requestJson<ApiTask[]>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/conversations/${encodeURIComponent(conversationId)}/tasks`,
    { headers: authHeaders(token) }
  )
}

export async function getTasksPaginated(
  teamId: string,
  conversationId: string,
  token: string,
  options?: GetTasksPaginatedOptions
): Promise<ApiTasksListResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  if (options?.executedOnly) params.set("executed_only", "true")
  const q = params.toString()
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/conversations/${encodeURIComponent(conversationId)}/tasks${q ? `?${q}` : ""}`
  return requestJson<ApiTasksListResponse>(url, { headers: authHeaders(token) })
}

export async function getTask(teamId: string, taskId: string, token: string): Promise<ApiTask> {
  return requestJson<ApiTask>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}`, {
    headers: authHeaders(token),
  })
}

export async function getTaskConversation(
  teamId: string,
  taskId: string,
  token: string
): Promise<ApiSession | null> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/conversation`,
    { headers: authHeaders(token) }
  )
  if (res.status === 404) return null
  await throwIfNotOk(res)
  return res.json() as Promise<ApiSession>
}

export async function createTask(
  teamId: string,
  body: CreateTaskBody,
  token: string
): Promise<ApiTask> {
  const conv = await createConversation(teamId, { channel: "portal" }, token)
  return requestJson<ApiTask>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/conversations/${encodeURIComponent(conv.conversation_id)}/tasks`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function createTaskRun(
  teamId: string,
  taskId: string,
  body: { input: string },
  token: string
): Promise<CreateTaskRunResponse> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/runs`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
  if (res.status === 409) {
    const msg = await parseErrorResponse(res, "A run is already in progress for this task")
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<CreateTaskRunResponse>
}

/**
 * Ask the server to stop the task's run.
 *
 * 409 means there was nothing to stop — the run finished between the page
 * rendering a Stop button and the click reaching the server — which is a state
 * the caller resolves by reloading, not an error worth showing.
 */
export async function cancelTask(
  teamId: string,
  taskId: string,
  token: string
): Promise<CancelTaskResponse> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/cancel`,
    { method: "POST", headers: { ...jsonHeaders, ...authHeaders(token) } }
  )
  if (res.status === 409) {
    const msg = await parseErrorResponse(res, "This task has no run in progress")
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<CancelTaskResponse>
}

export function subscribeTaskStream(
  teamId: string,
  taskId: string,
  token: string,
  callbacks: RunStreamCallbacks
): () => void {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/stream`
  const controller = new AbortController()

  void apiFetch(url, { headers: authHeaders(token), signal: controller.signal })
    .then(async (res) => {
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
