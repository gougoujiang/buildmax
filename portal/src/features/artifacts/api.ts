import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiArtifact, ApiArtifactItem } from "../../lib/api/types"

export async function getArtifacts(
  workspaceId: string,
  token: string,
  options?: { chatId?: string }
): Promise<ApiArtifact[]> {
  let url = `${getApiBase()}/api/workspaces/${workspaceId}/artifacts`
  if (options?.chatId) {
    url += `?task_id=${encodeURIComponent(options.chatId)}`
  }
  return requestJson<ApiArtifact[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactItems(
  workspaceId: string,
  chatRunId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(chatRunId)}/items`
  return requestJson<ApiArtifactItem[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactContent(
  workspaceId: string,
  chatRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/workspaces/${encodeURIComponent(workspaceId)}/artifacts/${encodeURIComponent(chatRunId)}/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
}
