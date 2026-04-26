import {
  checkUnauthorized,
  getApiBase,
  parseErrorResponse,
  requestJson,
  throwIfNotOk,
} from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiTeam, ApiTeamMember } from "../../lib/api/types"

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

export async function addTeamMember(
  teamId: string,
  body: { email: string; role?: string },
  token: string
): Promise<ApiTeamMember> {
  return requestJson<ApiTeamMember>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members`,
    {
      method: "POST",
      headers: { ...authHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  )
}

export async function removeTeamMember(
  teamId: string,
  userId: string,
  token: string
): Promise<void> {
  const res = await fetch(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/members/${encodeURIComponent(userId)}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    }
  )
  checkUnauthorized(res)
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Failed to remove member"))
  }
  await throwIfNotOk(res)
}
