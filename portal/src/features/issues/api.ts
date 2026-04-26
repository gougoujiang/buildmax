import {
  requestJson,
  getApiBase,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiIssue, ApiIssuesListResponse } from "../../lib/api/types"

export interface GetIssuesOptions {
  limit?: number
  offset?: number
}

export async function getIssues(teamId: string, token: string, options?: GetIssuesOptions): Promise<ApiIssuesListResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  const q = params.toString()
  return requestJson<ApiIssuesListResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues${q ? `?${q}` : ""}`, {
    headers: authHeaders(token),
  })
}

export async function getIssue(teamId: string, issueId: string, token: string): Promise<ApiIssue> {
  return requestJson<ApiIssue>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues/${encodeURIComponent(issueId)}`, {
    headers: authHeaders(token),
  })
}

export async function createIssue(
  teamId: string,
  body: { title: string; description?: string },
  token: string,
): Promise<ApiIssue> {
  return requestJson<ApiIssue>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function updateIssue(
  teamId: string,
  issueId: string,
  body: {
    title?: string
    description?: string
    status?: "todo" | "in_progress" | "done"
    assignee_kind?: "person" | "agent" | "workflow" | ""
    assignee_id?: string
  },
  token: string,
): Promise<ApiIssue> {
  return requestJson<ApiIssue>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues/${encodeURIComponent(issueId)}`, {
    method: "PATCH",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}
