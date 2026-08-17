import { apiFetch, requestJson, throwIfNotOk, getApiBase } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type { ApiIssueComment, ApiIssueCommentsResponse } from "../../lib/api/types"

function commentsBase(teamId: string, issueId: string): string {
  return `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues/${encodeURIComponent(issueId)}/comments`
}

export async function getIssueComments(
  teamId: string,
  issueId: string,
  token: string,
  options?: { limit?: number; offset?: number },
): Promise<ApiIssueCommentsResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  const q = params.toString()
  return requestJson<ApiIssueCommentsResponse>(`${commentsBase(teamId, issueId)}${q ? `?${q}` : ""}`, {
    headers: authHeaders(token),
  })
}

export async function createIssueComment(
  teamId: string,
  issueId: string,
  body: string,
  token: string,
): Promise<ApiIssueComment> {
  return requestJson<ApiIssueComment>(commentsBase(teamId, issueId), {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify({ body }),
  })
}

export async function updateIssueComment(
  teamId: string,
  issueId: string,
  commentId: string,
  body: string,
  token: string,
): Promise<ApiIssueComment> {
  return requestJson<ApiIssueComment>(`${commentsBase(teamId, issueId)}/${encodeURIComponent(commentId)}`, {
    method: "PATCH",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify({ body }),
  })
}

export async function deleteIssueComment(
  teamId: string,
  issueId: string,
  commentId: string,
  token: string,
): Promise<void> {
  const res = await apiFetch(`${commentsBase(teamId, issueId)}/${encodeURIComponent(commentId)}`, {
    method: "DELETE",
    headers: authHeaders(token),
  })
  await throwIfNotOk(res)
}
