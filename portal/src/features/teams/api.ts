import {
  apiFetch,
  getApiBase,
  parseErrorResponse,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type {
  ApiInvitation,
  ApiMemberLoginCode,
  ApiMemberRole,
  ApiTeam,
  ApiTeamMember,
  ApiUsage,
} from "../../lib/api/types"

export async function getTeams(token: string): Promise<ApiTeam[]> {
  return requestJson<ApiTeam[]>(`${getApiBase()}/api/teams`, {
    headers: authHeaders(token),
  })
}

export async function createTeam(
  body: { name: string },
  token: string
): Promise<ApiTeam> {
  return requestJson<ApiTeam>(`${getApiBase()}/api/teams`, {
    method: "POST",
    headers: { ...authHeaders(token), "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
}

export async function getTeamMembers(teamId: string, token: string): Promise<ApiTeamMember[]> {
  return requestJson<ApiTeamMember[]>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members`,
    {
      headers: authHeaders(token),
    }
  )
}

export async function getTeamUsage(teamId: string, token: string): Promise<ApiUsage> {
  return requestJson<ApiUsage>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/usage`,
    {
      headers: authHeaders(token),
    }
  )
}

export async function removeTeamMember(
  teamId: string,
  userId: string,
  token: string
): Promise<void> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    }
  )
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Failed to remove member"))
  }
  await throwIfNotOk(res)
}

/**
 * Change a member's role. Setting role to "owner" is ownership transfer: the
 * caller is demoted to admin in the same call, unilaterally and immediately.
 * See docs/design/team-membership-lifecycle.md.
 */
export async function setMemberRole(
  teamId: string,
  userId: string,
  body: { role: string },
  token: string
): Promise<ApiMemberRole> {
  return requestJson<ApiMemberRole>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
    {
      method: "PATCH",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  )
}

/**
 * Issue a login code for a locked-out member of the caller's own team. The
 * code is shown once and recorded nowhere -- see
 * docs/design/team-membership-lifecycle.md §5.4.
 */
export async function issueMemberLoginCode(
  teamId: string,
  userId: string,
  token: string
): Promise<ApiMemberLoginCode> {
  return requestJson<ApiMemberLoginCode>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}/login-code`,
    {
      method: "POST",
      headers: authHeaders(token),
    }
  )
}

/**
 * Invite an existing account to join the team. Bounded to an account that
 * already exists -- creating one is a system_admin operation, not a
 * team-scoped one. See docs/design/team-membership-lifecycle.md.
 */
export async function inviteMember(
  teamId: string,
  body: { email: string; role?: string },
  token: string
): Promise<ApiInvitation> {
  return requestJson<ApiInvitation>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/invitations`,
    {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  )
}

export async function getTeamInvitations(teamId: string, token: string): Promise<ApiInvitation[]> {
  return requestJson<ApiInvitation[]>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/invitations`,
    {
      headers: authHeaders(token),
    }
  )
}

export async function revokeInvitation(
  teamId: string,
  invitationId: string,
  token: string
): Promise<void> {
  const res = await apiFetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/invitations/${encodeURIComponent(invitationId)}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    }
  )
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Failed to revoke the invitation"))
  }
  await throwIfNotOk(res)
}

/** What is pending for the signed-in caller, across every team. Not team-scoped. */
export async function getMyInvitations(token: string): Promise<ApiInvitation[]> {
  return requestJson<ApiInvitation[]>(`${getApiBase()}/api/invitations`, {
    headers: authHeaders(token),
  })
}

/**
 * Accept a pending invitation. Takes no code: the caller already reached
 * this session on their own, so this is authorized by the invitation being
 * their own pending row.
 */
export async function acceptInvitation(
  invitationId: string,
  token: string
): Promise<ApiInvitation> {
  return requestJson<ApiInvitation>(
    `${getApiBase()}/api/invitations/${encodeURIComponent(invitationId)}/accept`,
    {
      method: "POST",
      headers: authHeaders(token),
    }
  )
}
