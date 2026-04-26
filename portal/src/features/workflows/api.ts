import { requestJson, getApiBase } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiWorkflow,
  ApiWorkflowListResponse,
  ApiWorkflowRunDetailResponse,
  ApiWorkflowRunListResponse,
} from "../../lib/api/types"

export async function getWorkflows(teamId: string, token: string): Promise<ApiWorkflowListResponse> {
  return requestJson<ApiWorkflowListResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows`, {
    headers: authHeaders(token),
  })
}

export async function getWorkflow(teamId: string, workflowId: string, token: string): Promise<ApiWorkflow> {
  return requestJson<ApiWorkflow>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows/${encodeURIComponent(workflowId)}`, {
    headers: authHeaders(token),
  })
}

export async function createWorkflow(
  teamId: string,
  body: { name: string; description?: string; definition: string },
  token: string,
): Promise<ApiWorkflow> {
  return requestJson<ApiWorkflow>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function updateWorkflow(
  teamId: string,
  workflowId: string,
  body: { name?: string; description?: string; definition?: string },
  token: string,
): Promise<ApiWorkflow> {
  return requestJson<ApiWorkflow>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows/${encodeURIComponent(workflowId)}`, {
    method: "PATCH",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function getWorkflowRuns(
  teamId: string,
  workflowId: string,
  token: string,
): Promise<ApiWorkflowRunListResponse> {
  return requestJson<ApiWorkflowRunListResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows/${encodeURIComponent(workflowId)}/runs`, {
    headers: authHeaders(token),
  })
}

export async function getWorkflowRunDetail(
  teamId: string,
  workflowRunId: string,
  token: string,
): Promise<ApiWorkflowRunDetailResponse> {
  return requestJson<ApiWorkflowRunDetailResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflow-runs/${encodeURIComponent(workflowRunId)}`, {
    headers: authHeaders(token),
  })
}

export async function runWorkflow(
  teamId: string,
  workflowId: string,
  token: string,
  issueId?: string,
): Promise<ApiWorkflowRunDetailResponse> {
  return requestJson<ApiWorkflowRunDetailResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/workflows/${encodeURIComponent(workflowId)}/runs`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(issueId ? { issue_id: issueId } : {}),
  })
}

export async function runIssueWorkflow(
  teamId: string,
  issueId: string,
  token: string,
): Promise<ApiWorkflowRunDetailResponse> {
  return requestJson<ApiWorkflowRunDetailResponse>(`${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/issues/${encodeURIComponent(issueId)}/workflow-runs`, {
    method: "POST",
    headers: authHeaders(token),
  })
}
