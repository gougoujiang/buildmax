import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiSpaceAgentInstructions } from "../../lib/api/types"

function base(spaceId: string): string {
  return `${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/agent-instructions`
}

export function getSpaceAgentInstructions(
  token: string,
  spaceId: string,
): Promise<ApiSpaceAgentInstructions> {
  return requestJson<ApiSpaceAgentInstructions>(base(spaceId), { headers: authHeaders(token) })
}

export function setSpaceAgentInstructions(
  token: string,
  spaceId: string,
  instructions: string,
): Promise<ApiSpaceAgentInstructions> {
  return requestJson<ApiSpaceAgentInstructions>(base(spaceId), {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ instructions }),
  })
}
