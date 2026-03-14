import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiWorkspace } from "../../lib/api/types"

export async function getWorkspaces(token: string): Promise<ApiWorkspace[]> {
  return requestJson<ApiWorkspace[]>(`${getApiBase()}/api/workspaces`, {
    headers: authHeaders(token),
  })
}

export async function createWorkspace(
  body: { name: string },
  token: string
): Promise<ApiWorkspace> {
  return requestJson<ApiWorkspace>(`${getApiBase()}/api/workspaces`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}
