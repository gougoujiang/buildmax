import {
  checkUnauthorized,
  getApiBase,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiAgent } from "../../lib/api/types"

export async function getAgents(
  workspaceId: string,
  token: string
): Promise<ApiAgent[]> {
  return requestJson<ApiAgent[]>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
    { headers: authHeaders(token) }
  )
}

export async function createAgent(
  workspaceId: string,
  body: { name: string; description?: string; instructions?: string },
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function updateAgent(
  workspaceId: string,
  agentId: string,
  body: { name: string; description?: string; instructions?: string },
  token: string
): Promise<ApiAgent> {
  return requestJson<ApiAgent>(
    `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents/${encodeURIComponent(agentId)}`,
    {
      method: "PATCH",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function deleteAgent(
  workspaceId: string,
  agentId: string,
  token: string
): Promise<void> {
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/agents/${encodeURIComponent(agentId)}`
  const res = await fetch(url, { method: "DELETE", headers: authHeaders(token) })
  checkUnauthorized(res)
  await throwIfNotOk(res)
}
