import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiPluginResponse, ApiPluginsResponse } from "../../lib/api/types"

/**
 * Client for /api/plugins, the browsable half of the Marketplace.
 *
 * These need an active account and nothing more. A release changes nothing
 * until somebody installs it deliberately, so reading the catalog is not a
 * privileged action; publishing to it is, and that lives under /api/admin.
 */

export function listPlugins(token: string): Promise<ApiPluginsResponse> {
  return requestJson<ApiPluginsResponse>(`${getApiBase()}/api/plugins`, {
    headers: authHeaders(token),
  })
}

/** One entry and every release under it, withdrawn ones included and marked. */
export function getPlugin(token: string, name: string): Promise<ApiPluginResponse> {
  return requestJson<ApiPluginResponse>(
    `${getApiBase()}/api/plugins/${encodeURIComponent(name)}`,
    { headers: authHeaders(token) },
  )
}
