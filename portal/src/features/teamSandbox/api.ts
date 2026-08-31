import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiTeamSandboxDefaults } from "../../lib/api/types"

/**
 * Client for a team's default sandbox tiers -- the tiers an agent that
 * declares neither inherits. Reading needs membership; changing needs owner
 * or admin. See docs/design/agent-sandbox-policy.md §9 M3.
 */

function base(teamId: string): string {
  return `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/sandbox-defaults`
}

export function getSandboxDefaults(token: string, teamId: string): Promise<ApiTeamSandboxDefaults> {
  return requestJson<ApiTeamSandboxDefaults>(base(teamId), { headers: authHeaders(token) })
}

export function setSandboxDefaults(
  token: string,
  teamId: string,
  defaults: ApiTeamSandboxDefaults,
): Promise<ApiTeamSandboxDefaults> {
  return requestJson<ApiTeamSandboxDefaults>(base(teamId), {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(defaults),
  })
}
