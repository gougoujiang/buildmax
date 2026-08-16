import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiAuditEventsResponse } from "../../lib/api/types"

/**
 * Fetch a page of a team's audit trail, newest first.
 *
 * Owner only: a 403 here is the expected answer for anyone else, not a bug.
 */
export async function getAuditEvents(
  teamId: string,
  token: string,
  options?: { limit?: number; offset?: number }
): Promise<ApiAuditEventsResponse> {
  const params = new URLSearchParams()
  if (options?.limit) params.set("limit", String(options.limit))
  if (options?.offset) params.set("offset", String(options.offset))
  const query = params.toString()
  const url =
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/audit-events` +
    (query ? `?${query}` : "")
  return requestJson<ApiAuditEventsResponse>(url, { headers: authHeaders(token) })
}
