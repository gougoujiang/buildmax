import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiRunProvenance, ApiTaskRunLLMCall, ApiTaskRunTrace } from "../../lib/api/types"

/**
 * Fetch a task run's trace summary.
 *
 * A 404 is an ordinary outcome, not a bug: a run that failed before an agent
 * started recorded no trace, and an old run may have had its trace expire from
 * storage. The server distinguishes the two in its message, so callers should
 * show that message rather than a generic failure.
 */
export async function getTaskRunTrace(
  teamId: string,
  taskRunId: string,
  token: string
): Promise<ApiTaskRunTrace> {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}/trace`
  return requestJson<ApiTaskRunTrace>(url, { headers: authHeaders(token) })
}

/**
 * Fetch the managed model calls one task run made.
 *
 * An empty list is an ordinary answer, not an error: a deployment in direct
 * mode never routes a worker's calls through the server, so there is nothing to
 * account. A 503 means this deployment records no managed calls at all. The
 * caller distinguishes the two — see `describeSpend`.
 */
export async function listTaskRunLLMCalls(
  teamId: string,
  taskRunId: string,
  token: string
): Promise<ApiTaskRunLLMCall[]> {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}/llm-calls`
  return requestJson<ApiTaskRunLLMCall[]>(url, { headers: authHeaders(token) })
}

/**
 * Fetch where one task run came from.
 *
 * Separate from the trace because it answers a different question and survives
 * a different absence: a run that failed before an agent started has no trace
 * at all, and still came from somewhere.
 */
export async function getTaskRunProvenance(
  teamId: string,
  taskRunId: string,
  token: string
): Promise<ApiRunProvenance> {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}`
  return requestJson<ApiRunProvenance>(url, { headers: authHeaders(token) })
}
