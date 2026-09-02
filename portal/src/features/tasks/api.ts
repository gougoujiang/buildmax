import {
  apiFetch,
  getApiBase,
  parseErrorResponse,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiTask,
  ApiTaskRun,
  ApiTaskRunsResponse,
  ApiTasksListResponse,
  CancelTaskResponse,
  RetryTaskResponse,
} from "../../lib/api/types"

export interface GetTasksPaginatedOptions {
  limit?: number
  offset?: number
  executedOnly?: boolean
}

export async function createAgentTask(teamId: string, agentId: string, input: string, token: string): Promise<ApiTask> {
  return requestJson<ApiTask>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}/tasks`,
    { method: "POST", headers: { ...jsonHeaders, ...authHeaders(token) }, body: JSON.stringify({ input }) }
  )
}

export async function listAgentTasks(teamId: string, agentId: string, token: string): Promise<ApiTasksListResponse> {
  return requestJson<ApiTasksListResponse>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}/tasks`,
    { headers: authHeaders(token) }
  )
}

export async function getTask(teamId: string, taskId: string, token: string): Promise<ApiTask> {
  return requestJson<ApiTask>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}`,
    { headers: authHeaders(token) }
  )
}

export async function getTaskRuns(teamId: string, taskId: string, token: string): Promise<ApiTaskRun[]> {
  const response = await requestJson<ApiTaskRunsResponse>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/runs`,
    { headers: authHeaders(token) }
  )
  return response.runs
}

/**
 * Continue a task with a new input.
 *
 * idempotencyKey lets a caller that cannot tell whether an earlier attempt
 * landed retry safely: the server returns the run the first attempt created
 * instead of starting a second one. Optional so a caller with no retry logic
 * of its own is unaffected.
 */
export async function continueTask(
  teamId: string,
  taskId: string,
  input: string,
  token: string,
  idempotencyKey?: string
): Promise<ApiTaskRun> {
  return requestJson<ApiTaskRun>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/runs`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify({ input, idempotency_key: idempotencyKey }),
    }
  )
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

/**
 * Run the task's last run again, with the input that run carried.
 *
 * 409 covers three different states — a run already in flight, a task that has
 * never finished one, and a workflow step, which the workflow owns — so the
 * server's own reason is what the caller shows.
 */
export async function retryTask(
  teamId: string,
  taskId: string,
  token: string
): Promise<RetryTaskResponse> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(taskId)}/retry`,
    { method: "POST", headers: { ...jsonHeaders, ...authHeaders(token) } }
  )
  if (res.status === 409) {
    const msg = await parseErrorResponse(res, "This task cannot be retried right now")
    throw new Error(msg)
  }
  await throwIfNotOk(res)
  return res.json() as Promise<RetryTaskResponse>
}
