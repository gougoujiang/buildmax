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

// listAgentModels returns the model names the deployment offers, for the agent
// editor's model picker. The endpoint is deployment-wide (any signed-in user),
// so it takes no team. A deployment with no catalog yields an empty list, which
// the picker renders as just the deployment default.
export async function listAgentModels(token: string): Promise<string[]> {
  const res = await requestJson<{ models?: Array<{ name: string }> }>(
    `${getApiBase()}/api/llm/models`,
    { headers: authHeaders(token) },
  )
  return (res.models ?? []).map((m) => m.name)
}

export async function getAgents(teamId: string, token: string): Promise<ApiAgent[]> {
  return requestJson<ApiAgent[]>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents`, { headers: authHeaders(token) })
}

export async function getAgent(teamId: string, agentId: string, token: string): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/agents/${encodeURIComponent(agentId)}`,
    { headers: authHeaders(token) },
  )
}

export async function createAgent(
  teamId: string,
  body: {
    name: string
    description?: string
    instructions?: string
    model?: string
    plugins?: string[]
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
    model?: string
    plugins?: string[]
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
