import { getApiBase, requestJson, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { UploadResponse } from "../../lib/api/types"
import type { ExploreNode } from "../../lib/types"

export async function uploadFiles(
  files: File[],
  spaceId: string,
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
  return requestJson<UploadResponse>(`${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/upload`, {
    method: "POST",
    headers: authHeaders(token),
    body: formData,
  })
}

export async function getFileTree(spaceId: string, token: string): Promise<ExploreNode> {
  return requestJson<ExploreNode>(`${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/files`, {
    headers: authHeaders(token),
  })
}

export async function getFileContent(
  spaceId: string,
  filePath: string,
  token: string
): Promise<string> {
  const encodedPath = filePath.split("/").map(encodeURIComponent).join("/")
  return requestText(`${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/files/${encodedPath}`, {
    headers: authHeaders(token),
  })
}
