import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { UploadResponse } from "../../lib/api/types"
import type { ExploreNode } from "../../lib/types"

export async function uploadFiles(
  files: File[],
  teamId: string,
  token: string,
  paths?: string[]
): Promise<UploadResponse> {
  const formData = new FormData()
  for (const file of files) {
    formData.append("files", file)
  }
  if (paths) {
    for (const path of paths) {
      formData.append("paths", path)
    }
  }
  return requestJson<UploadResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/upload`, {
    method: "POST",
    headers: authHeaders(token),
    body: formData,
  })
}

export async function getFileTree(teamId: string, token: string): Promise<ExploreNode> {
  return requestJson<ExploreNode>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/files`, {
    headers: authHeaders(token),
  })
}

export async function getFileContent(
  teamId: string,
  filePath: string,
  token: string
): Promise<string> {
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/")
  return requestText(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/files/${encodedPath}`, {
    headers: authHeaders(token),
  })
}
