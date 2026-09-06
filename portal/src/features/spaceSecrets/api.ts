import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiCreateSecretRequest,
  ApiEditSecretRequest,
  ApiSecret,
  ApiSecretListResponse,
  ApiSetSecretStateRequest,
} from "../../lib/api/types"

/**
 * Client for a space's Secrets. Every route is owner-only, and values are
 * write-only: create and edit send item values, but nothing here reads one
 * back. See docs/design/space-secrets.md.
 */

function base(spaceId: string): string {
  return `${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/secrets`
}

function one(spaceId: string, secretId: string): string {
  return `${base(spaceId)}/${encodeURIComponent(secretId)}`
}

export function listSecrets(token: string, spaceId: string): Promise<ApiSecretListResponse> {
  return requestJson<ApiSecretListResponse>(base(spaceId), { headers: authHeaders(token) })
}

export function createSecret(
  token: string,
  spaceId: string,
  req: ApiCreateSecretRequest,
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(base(spaceId), {
    method: "POST",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(req),
  })
}

export function editSecret(
  token: string,
  spaceId: string,
  secretId: string,
  req: ApiEditSecretRequest,
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(one(spaceId, secretId), {
    method: "PATCH",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(req),
  })
}

export function setSecretState(
  token: string,
  spaceId: string,
  secretId: string,
  state: ApiSetSecretStateRequest["state"],
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(`${one(spaceId, secretId)}/state`, {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ state } satisfies ApiSetSecretStateRequest),
  })
}
