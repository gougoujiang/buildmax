import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiArtifact, ApiArtifactItem } from "../../lib/api/types"

export async function getArtifacts(
  teamId: string,
  token: string,
  options?: { taskId?: string }
): Promise<ApiArtifact[]> {
  if (!options?.taskId) {
    return []
  }
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/tasks/${encodeURIComponent(options.taskId)}/artifacts`
  return requestJson<ApiArtifact[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactItems(
  teamId: string,
  taskRunId: string,
  token: string
): Promise<ApiArtifactItem[]> {
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}/artifacts/items`
  return requestJson<ApiArtifactItem[]>(url, { headers: authHeaders(token) })
}

export async function getArtifactContent(
  teamId: string,
  taskRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}/artifacts/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
}
