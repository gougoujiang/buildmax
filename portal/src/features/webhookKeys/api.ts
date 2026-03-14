import {
  getApiBase,
  requestJson,
  checkUnauthorized,
  parseErrorResponse,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"

export interface WebhookKeyMeta {
  key_id: string
  name: string
  created_at: number
}

export interface CreateWebhookKeyResponse {
  key: string
  key_id: string
}

export interface ListWebhookKeysResponse {
  keys: WebhookKeyMeta[]
}

export async function createWebhookKey(
  workspaceId: string,
  body: { name?: string },
  token: string
): Promise<CreateWebhookKeyResponse> {
  return requestJson<CreateWebhookKeyResponse>(
    `${getApiBase()}/api/workspaces/${workspaceId}/webhook-keys`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}

export async function listWebhookKeys(
  workspaceId: string,
  token: string
): Promise<ListWebhookKeysResponse> {
  return requestJson<ListWebhookKeysResponse>(
    `${getApiBase()}/api/workspaces/${workspaceId}/webhook-keys`,
    { headers: authHeaders(token) }
  )
}

export async function revokeWebhookKey(
  workspaceId: string,
  keyId: string,
  token: string
): Promise<void> {
  const res = await fetch(
    `${getApiBase()}/api/workspaces/${workspaceId}/webhook-keys/${keyId}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    }
  )
  checkUnauthorized(res)
  if (!res.ok) {
    const msg = await parseErrorResponse(res, "Failed to revoke key")
    throw new Error(msg)
  }
}
