import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiArtifact, ApiArtifactItem } from "../../lib/api/types"

export async function getArtifacts(
  _profileId: string,
  token: string,
  options?: { taskId?: string }
): Promise<ApiArtifact[]> {
  if (!options?.taskId) {
    return []
  }
  const url = `${getApiBase()}/api/tasks/${encodeURIComponent(options.taskId)}/artifacts`
  return requestJson<ApiArtifact[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactItems(
  taskRunId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/task-runs/${encodeURIComponent(taskRunId)}/artifacts/items`
  return requestJson<ApiArtifactItem[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactContent(
  taskRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/task-runs/${encodeURIComponent(taskRunId)}/artifacts/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
}
