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
  body: { name?: string },
  token: string
): Promise<CreateWebhookKeyResponse> {
  return requestJson<CreateWebhookKeyResponse>(`${getApiBase()}/api/webhook-keys`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function listWebhookKeys(
  token: string
): Promise<ListWebhookKeysResponse> {
  return requestJson<ListWebhookKeysResponse>(`${getApiBase()}/api/webhook-keys`, {
    headers: authHeaders(token),
  })
}

export async function revokeWebhookKey(
  keyId: string,
  token: string
): Promise<void> {
  const res = await fetch(`${getApiBase()}/api/webhook-keys/${keyId}`, {
    method: "DELETE",
    headers: authHeaders(token),
  })
  checkUnauthorized(res)
  if (!res.ok) {
    const msg = await parseErrorResponse(res, "Failed to revoke key")
    throw new Error(msg)
  }
}
