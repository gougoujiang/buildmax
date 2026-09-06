import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiSpaceSandboxDefaults } from "../../lib/api/types"

/**
 * Client for a space's default sandbox tiers -- the tiers an agent that
 * declares neither inherits. Reading needs membership; changing needs owner
 * or admin. See docs/design/agent-sandbox-policy.md §9 M3.
 */

function base(spaceId: string): string {
  return `${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/sandbox-defaults`
}

export function getSandboxDefaults(token: string, spaceId: string): Promise<ApiSpaceSandboxDefaults> {
  return requestJson<ApiSpaceSandboxDefaults>(base(spaceId), { headers: authHeaders(token) })
}

export function setSandboxDefaults(
  token: string,
  spaceId: string,
  defaults: ApiSpaceSandboxDefaults,
): Promise<ApiSpaceSandboxDefaults> {
  return requestJson<ApiSpaceSandboxDefaults>(base(spaceId), {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(defaults),
  })
}
