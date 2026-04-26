/**
 * API request/response types (snake_case as returned by the server).
 * Single source of truth for DTOs; see design/004-portal-api-contract.md for full contract.
 */

export interface LoginUser {
  id: string
  email: string
  name: string
}

export interface LoginResponse {
  token: string
  user: LoginUser
}

export interface OtpRequestResponse {
  message: string
}

/** Legacy: user is the top-level owner. Kept for API compatibility. */
export interface ApiWorkspace {
  id: string
  name: string
  owner_user_id?: string
  created_at?: number
}

/** Agent as returned by team-scoped agent endpoints. */
export interface ApiAgent {
  id: string
  user_id: string
  team_id: string
  name: string
  description: string
  instructions: string
  created_at: number
}

export interface ApiIssue {
  id: string
  user_id: string
  team_id: string
  title: string
  description: string
  status: string
  assignee_kind?: string | null
  assignee_id?: string | null
  created_by: string
  created_at: number
  updated_at: number
}

export interface ApiWorkflow {
  id: string
  team_id: string
  name: string
  description: string
  definition: string
  status: string
  created_by: string
  created_at: number
  updated_at: number
}

export interface ApiWorkflowListResponse {
  workflows: ApiWorkflow[]
}

export interface ApiWorkflowRun {
  id: string
  workflow_id: string
  issue_id?: string | null
  conversation_id: string
  status: string
  created_by: string
  created_at: number
  started_at?: number | null
  ended_at?: number | null
  error_message?: string | null
}

export interface ApiWorkflowStepRun {
  id: string
  workflow_run_id: string
  step_id: string
  step_index: number
  step_type: string
  target_agent_id?: string | null
  prompt: string
  status: string
  task_id?: string | null
  task_run_id?: string | null
  output_summary?: string | null
  error_message?: string | null
  created_at: number
  started_at?: number | null
  ended_at?: number | null
}

export interface ApiWorkflowRunListResponse {
  runs: ApiWorkflowRun[]
  total: number
}

export interface ApiWorkflowRunDetailResponse {
  run: ApiWorkflowRun
  steps: ApiWorkflowStepRun[]
}

export interface ApiIssueFlowRun {
  run: ApiWorkflowRun
  steps: ApiWorkflowStepRun[]
}

export interface ApiIssueFlowResponse {
  issue: ApiIssue
  workflow?: ApiWorkflow | null
  runs: ApiIssueFlowRun[]
  agent_tasks: ApiTask[]
  total: number
}

export interface ApiIssuesListResponse {
  issues: ApiIssue[]
  total: number
}

/** Paginated tasks response when using limit/offset/executed_only (if backend supports it). */
export interface ApiTasksListResponse {
  tasks: ApiTask[]
  total: number
}

/** Task as returned by team-scoped task endpoints. */
export interface ApiTask {
  id: string
  conversation_id: string
  session_id: string | null
  status: string
  input: string
  title?: string
  output: string | null
  created_by: string
  created_at: number
  started_at: number | null
  ended_at: number | null
  error_message: string | null
  agent_id?: string | null
  issue_id?: string | null
}

/** Conversation as returned by the team-scoped task conversation endpoint. */
export interface ApiSession {
  id: string
  title: string
  created_at: string
  messages: ApiSessionMessage[]
}

export interface ApiSessionMessage {
  role: string
  content: string
  tool_call_id?: string
  tool_calls?: { id: string; name: string; arguments?: string }[]
}

/** Response from the team-scoped create task run endpoint. */
export interface CreateTaskRunResponse {
  task_run_id: string
  task_id: string
}

/** Run output (artifact) as returned by task/run artifact endpoints */
export interface ApiArtifact {
  task_run_id: string
  task_id: string
  conversation_id: string
  user_id: string
  created_at: number
  task_input_snippet: string
}

/** Artifact item as returned by the team-scoped task-run artifact items endpoint. */
export interface ApiArtifactItem {
  relative_path: string
}

/** Upload response from POST /api/upload */
export interface UploadResponse {
  uploaded: string[]
}

/** Usage as returned by GET /api/usage */
export interface ApiUsage {
  run_count: number
  total_tokens: number
  tier: string
  period_days: number
  max_runs_per_period?: number
  max_tokens_per_period?: number
}

/** Tier 1 conversation as returned by team-scoped conversation endpoints. */
export interface ApiConversation {
  id: string
  user_id: string
  team_id: string
  channel: string
  title?: string
  created_at: number
  created_by: string
}

export interface ApiTeam {
  id: string
  name: string
  personal_for_user_id?: string | null
  created_at?: number
}

export interface ApiTeamMember {
  team_id: string
  user_id: string
  role: string
  created_at?: number
  user_name?: string
  user_email?: string
}

/** Response from the team-scoped list conversations endpoint. */
export interface ApiConversationsListResponse {
  conversations: ApiConversation[]
  total: number
}

/** Response from the team-scoped create conversation endpoint. */
export interface CreateConversationResponse {
  conversation_id: string
  reply?: string
}

/** Message as returned by the team-scoped list conversation messages endpoint. */
export interface ApiConversationMessage {
  id: string
  role: string
  content: string
  channel?: string | null
  created_at: number
}

/** Response from the team-scoped list conversation messages endpoint. */
export interface ApiConversationMessagesResponse {
  messages: ApiConversationMessage[]
}

/** Response from the team-scoped add conversation message endpoint. */
export interface AddConversationMessageResponse {
  reply: string
}
