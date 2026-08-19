import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import { downloadAuthenticated } from "../../lib/download"
import type {
  ApiAdminGrantsResponse,
  ApiAdminLoginCode,
  ApiAdminMe,
  ApiAdminModel,
  ApiAdminModelsResponse,
  ApiAdminSessionsRevoked,
  ApiAdminSystem,
  ApiAdminTeamDetail,
  ApiAdminTeamsResponse,
  ApiAdminUser,
  ApiAdminUserAfterDisable,
  ApiAdminUserDetail,
  ApiAdminUsersResponse,
  ApiAuditEventsResponse,
  ApiSystemGrant,
} from "../../lib/api/types"

/**
 * Client for /api/admin.
 *
 * Every call here 403s for anyone without a deployment-scoped grant, and that
 * is the expected answer rather than a bug — the same convention the
 * team-scoped audit client already documents. Hiding the navigation is
 * presentation; the server refuses either way.
 */

function adminUrl(path: string, params?: Record<string, string | number | undefined>): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined && value !== "") query.set(key, String(value))
  }
  const suffix = query.toString()
  return `${getApiBase()}/api/admin${path}${suffix ? `?${suffix}` : ""}`
}

function get<T>(path: string, token: string, params?: Record<string, string | number | undefined>): Promise<T> {
  return requestJson<T>(adminUrl(path, params), { headers: authHeaders(token) })
}

function send<T>(method: string, path: string, token: string, body?: unknown): Promise<T> {
  return requestJson<T>(adminUrl(path), {
    method,
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

/** Whether the caller may operate this deployment. A rejection means no. */
export function getAdminMe(token: string): Promise<ApiAdminMe> {
  return get<ApiAdminMe>("/me", token)
}

export function getAdminSystem(token: string): Promise<ApiAdminSystem> {
  return get<ApiAdminSystem>("/system", token)
}

/** The effective server.yaml, with every credential reduced to whether it is set. */
export function getAdminConfig(token: string): Promise<Record<string, unknown>> {
  return get<Record<string, unknown>>("/config", token)
}

export function listAdminUsers(
  token: string,
  options?: { q?: string; limit?: number; offset?: number },
): Promise<ApiAdminUsersResponse> {
  return get<ApiAdminUsersResponse>("/users", token, options)
}

export function getAdminUser(token: string, userId: string): Promise<ApiAdminUserDetail> {
  return get<ApiAdminUserDetail>(`/users/${encodeURIComponent(userId)}`, token)
}

export function createAdminUser(token: string, email: string): Promise<ApiAdminUser> {
  return send<ApiAdminUser>("POST", "/users", token, { email })
}

/** Issues a single-use code. It is returned once and is recoverable nowhere. */
export function issueAdminLoginCode(token: string, userId: string): Promise<ApiAdminLoginCode> {
  return send<ApiAdminLoginCode>("POST", `/users/${encodeURIComponent(userId)}/login-code`, token)
}

export function setAdminUserDisabled(
  token: string,
  userId: string,
  disabled: boolean,
): Promise<ApiAdminUserAfterDisable> {
  const action = disabled ? "disable" : "enable"
  return send<ApiAdminUserAfterDisable>("POST", `/users/${encodeURIComponent(userId)}/${action}`, token)
}

export function revokeAdminUserSessions(token: string, userId: string): Promise<ApiAdminSessionsRevoked> {
  return send<ApiAdminSessionsRevoked>("DELETE", `/users/${encodeURIComponent(userId)}/sessions`, token)
}

export function listAdminGrants(token: string, includeRevoked = false): Promise<ApiAdminGrantsResponse> {
  return get<ApiAdminGrantsResponse>("/grants", token, {
    include_revoked: includeRevoked ? "true" : undefined,
  })
}

export function grantAdminRole(token: string, userId: string): Promise<ApiSystemGrant> {
  return send<ApiSystemGrant>("POST", "/grants", token, { user_id: userId })
}

/**
 * Revokes a grant. The server refuses to revoke the deployment's last one —
 * that is what `buildmax-server admin revoke` is for — so a 409 here is a
 * boundary doing its job.
 */
export function revokeAdminRole(token: string, userId: string): Promise<void> {
  return send<void>("DELETE", `/grants/${encodeURIComponent(userId)}`, token)
}

export function listAdminTeams(
  token: string,
  options?: { q?: string; limit?: number; offset?: number },
): Promise<ApiAdminTeamsResponse> {
  return get<ApiAdminTeamsResponse>("/teams", token, options)
}

export function getAdminTeam(token: string, teamId: string): Promise<ApiAdminTeamDetail> {
  return get<ApiAdminTeamDetail>(`/teams/${encodeURIComponent(teamId)}`, token)
}

export function searchAdminAuditEvents(
  token: string,
  options?: {
    team_id?: string
    actor_id?: string
    action?: string
    since?: number
    until?: number
    limit?: number
    offset?: number
  },
): Promise<ApiAuditEventsResponse> {
  return get<ApiAuditEventsResponse>("/audit-events", token, options)
}

/**
 * Download the deployment-wide trail as a file, under the same filters as the
 * search above.
 *
 * The server records the export, and records it against the administrator who
 * took it. An export narrowed to one space is recorded in that space's trail
 * too, so its owner can see that the deployment read their record.
 */
export async function exportAdminAuditEvents(
  token: string,
  format: "csv" | "jsonl",
  options?: { team_id?: string; actor_id?: string; action?: string; since?: number; until?: number },
): Promise<void> {
  const url = adminUrl("/audit-events/export", { ...options, format })
  await downloadAuthenticated(url, token, `audit-deployment.${format}`)
}

export function listAdminModels(token: string): Promise<ApiAdminModelsResponse> {
  return get<ApiAdminModelsResponse>("/llm/models", token)
}

/**
 * Retires or restores a catalog model.
 *
 * There is no create here on purpose: adding a model means handing over a
 * provider key, and `buildmax-server model add` keeps that on the machine that
 * already holds the database credentials rather than putting it in a request
 * body, a proxy log, and whatever the browser did with the form.
 */
export function setAdminModelEnabled(
  token: string,
  modelId: string,
  enabled: boolean,
): Promise<ApiAdminModel> {
  const action = enabled ? "enable" : "disable"
  return send<ApiAdminModel>("POST", `/llm/models/${encodeURIComponent(modelId)}/${action}`, token)
}
