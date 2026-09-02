import {
  apiFetch,
  getApiBase,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiAgent,
  ApiAgentRevisionListResponse,
  ApiSecretConsumption,
} from "../../lib/api/types"

export async function getAgents(teamId: string, token: string): Promise<ApiAgent[]> {
  return requestJson<ApiAgent[]>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents`, { headers: authHeaders(token) })
}

export async function createAgent(
  teamId: string,
  body: {
    name: string
    description?: string
    instructions?: string
    sandbox_network_tier?: string
    sandbox_filesystem_tier?: string
    secret_consumption?: ApiSecretConsumption
  },
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function updateAgent(
  teamId: string,
  agentId: string,
  body: {
    name: string
    description?: string
    instructions?: string
    sandbox_network_tier?: string
    sandbox_filesystem_tier?: string
    secret_consumption?: ApiSecretConsumption
  },
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}`,
    {
      method: "PATCH",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function deleteAgent(
  teamId: string,
  agentId: string,
  token: string
): Promise<void> {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}`
  const res = await apiFetch(url, { method: "DELETE", headers: authHeaders(token) })
  await throwIfNotOk(res)
}

export async function getAgentRevisions(
  teamId: string,
  agentId: string,
  token: string
): Promise<ApiAgentRevisionListResponse> {
  return requestJson<ApiAgentRevisionListResponse>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}/revisions`,
    { headers: authHeaders(token) }
  )
}

export async function restoreAgentRevision(
  teamId: string,
  agentId: string,
  revision: number,
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}/revisions/${revision}/restore`,
    { method: "POST", headers: authHeaders(token) }
  )
}
