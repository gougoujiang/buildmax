import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { UploadResponse } from "../../lib/api/types"
import type { ExploreNode } from "../../lib/types"

export async function uploadFiles(
  _profileId: string,
  files: File[],
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
  return requestJson<UploadResponse>(`${getApiBase()}/api/upload`, {
    method: "POST",
    headers: authHeaders(token),
    body: formData,
  })
}

export async function getFileTree(_profileId: string, token: string): Promise<ExploreNode> {
  return requestJson<ExploreNode>(`${getApiBase()}/api/files`, {
    headers: authHeaders(token),
  })
}

export async function getFileContent(
  _profileId: string,
  filePath: string,
  token: string
): Promise<string> {
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/")
  return requestText(`${getApiBase()}/api/files/${encodedPath}`, { headers: authHeaders(token) })
}
