import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiArtifact, ApiArtifactItem } from "../../lib/api/types"

export async function getArtifacts(
  _profileId: string,
  token: string,
  options?: { chatId?: string }
): Promise<ApiArtifact[]> {
  if (!options?.chatId) {
    return []
  }
  const url = `${getApiBase()}/api/tasks/${encodeURIComponent(options.chatId)}/artifacts`
  return requestJson<ApiArtifact[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactItems(
  chatRunId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/task-runs/${encodeURIComponent(chatRunId)}/artifacts/items`
  return requestJson<ApiArtifactItem[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactContent(
  chatRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/task-runs/${encodeURIComponent(chatRunId)}/artifacts/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
}
