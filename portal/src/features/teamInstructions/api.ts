import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiTeamAgentInstructions } from "../../lib/api/types"

function base(teamId: string): string {
  return `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agent-instructions`
}

export function getTeamAgentInstructions(
  token: string,
  teamId: string,
): Promise<ApiTeamAgentInstructions> {
  return requestJson<ApiTeamAgentInstructions>(base(teamId), { headers: authHeaders(token) })
}

export function setTeamAgentInstructions(
  token: string,
  teamId: string,
  instructions: string,
): Promise<ApiTeamAgentInstructions> {
  return requestJson<ApiTeamAgentInstructions>(base(teamId), {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ instructions }),
  })
}
