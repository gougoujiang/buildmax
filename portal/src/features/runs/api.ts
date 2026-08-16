import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiTaskRunTrace } from "../../lib/api/types"

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
