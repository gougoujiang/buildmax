import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiAuditEventsResponse } from "../../lib/api/types"
import { downloadAuthenticated } from "../../lib/download"

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

/**
 * Download a team's whole audit trail as a file.
 *
 * The export is the trail, not a page of it, and the server records that it was
 * taken — reading the whole record is itself an action on it.
 */
export async function exportAuditEvents(
  teamId: string,
  token: string,
  format: "csv" | "jsonl" = "csv"
): Promise<void> {
  const url =
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/audit-events/export` +
    `?format=${format}`
  await downloadAuthenticated(url, token, `audit-${teamId}.${format}`)
}
