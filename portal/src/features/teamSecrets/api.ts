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
 * Client for a team's Secrets. Every route is owner-only, and values are
 * write-only: create and edit send item values, but nothing here reads one
 * back. See docs/design/team-secrets.md.
 */

function base(teamId: string): string {
  return `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/secrets`
}

function one(teamId: string, secretId: string): string {
  return `${base(teamId)}/${encodeURIComponent(secretId)}`
}

export function listSecrets(token: string, teamId: string): Promise<ApiSecretListResponse> {
  return requestJson<ApiSecretListResponse>(base(teamId), { headers: authHeaders(token) })
}

export function createSecret(
  token: string,
  teamId: string,
  req: ApiCreateSecretRequest,
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(base(teamId), {
    method: "POST",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(req),
  })
}

export function editSecret(
  token: string,
  teamId: string,
  secretId: string,
  req: ApiEditSecretRequest,
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(one(teamId, secretId), {
    method: "PATCH",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify(req),
  })
}

export function setSecretState(
  token: string,
  teamId: string,
  secretId: string,
  state: ApiSetSecretStateRequest["state"],
): Promise<ApiSecret> {
  return requestJson<ApiSecret>(`${one(teamId, secretId)}/state`, {
    method: "PUT",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ state } satisfies ApiSetSecretStateRequest),
  })
}
