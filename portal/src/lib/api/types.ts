/**
 * API request/response types (snake_case as returned by the server).
 * Single source of truth for DTOs; see design/archive/004-portal-api-contract.md for historical contract context.
 */

export interface LoginUser {
  id: string
  email: string
  name: string
}

export interface LoginResponse {
  /** The access token under the name it had before the credentials were split. */
  token: string
  access_token?: string
  /** Absent when the deployment has no store to keep refresh tokens in. */
  refresh_token?: string
  /** Access token lifetime in seconds. */
  expires_in?: number
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
  parent_issue_id?: string | null
  title: string
  description: string
  status: string
  assignee_kind?: string | null
  assignee_id?: string | null
  created_by: string
  created_at: number
  updated_at: number
  /** Derived per response, never stored. Absent on older servers. */
  child_count?: number
  done_child_count?: number
  comment_count?: number
}

export interface ApiIssueComment {
  id: string
  issue_id: string
  author_kind: "user" | "agent" | "system"
  author_id: string
  body: string
  source_task_id?: string | null
  source_task_run_id?: string | null
  created_at: number
  /** Absent until the comment is edited. */
  edited_at?: number | null
}

export interface ApiIssueCommentsResponse {
  comments: ApiIssueComment[]
  total: number
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

export interface ApiOutputSource {
  source_type: string
  task_id?: string
  task_run_id?: string
  conversation_id?: string
  workflow_run_id?: string | null
  workflow_step_run_id?: string | null
  workflow_step_id?: string | null
}

export interface ApiIssueOutput {
  id: string
  title: string
  kind: string
  relative_path?: string
  preview?: string
  preview_truncated: boolean
  source: ApiOutputSource
  created_at: number
}

export interface ApiIssueFlowResponse {
  issue: ApiIssue
  /** Set on a sub-issue; children is set on a parent. Never both. */
  parent?: ApiIssue | null
  children: ApiIssue[]
  workflow?: ApiWorkflow | null
  runs: ApiIssueFlowRun[]
  agent_tasks: ApiTask[]
  latest_result?: ApiIssueOutput | null
  outputs: ApiIssueOutput[]
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

/** The execution boundary a run actually ran under. */
export interface ApiTraceBoundary {
  /** False means nothing confined the run's shell commands. Never assume true. */
  sandboxed: boolean
  mode?: string
  backend?: string
  /** The layer chain that decided the boundary, e.g. ["default:worker", "policy"]. */
  sources?: string[]
  downgraded?: boolean
}

/** One tool call in a run. */
export interface ApiTraceToolCall {
  name: string
  duration_ms?: number
  /** The call's file_path argument, when it had one. */
  path?: string
  denied?: boolean
  deny_reason?: string
}

/**
 * A task run's trace summary. The server deliberately omits model output, tool
 * arguments, and tool results — this describes the shape of a run, not its
 * content.
 */
export interface ApiTaskRunTrace {
  task_run_id: string
  run_id?: string
  model?: string
  started_at?: string
  ended_at?: string
  boundary?: ApiTraceBoundary
  llm_calls: number
  tool_calls: number
  compactions: number
  prompt_tokens: number
  completion_tokens: number
  tools?: ApiTraceToolCall[]
  /** The tools list was bounded; tool_calls still counts them all. */
  tools_truncated?: boolean
  files_changed?: string[]
  /** Terminal error; empty when the run succeeded. */
  error?: string
  /** False means the run wrote no terminal record. Do not read it as success. */
  complete: boolean
}

/**
 * One recorded action. The server records that something happened and who did
 * it — never prompts, generated content, tool output, or credentials.
 */
export interface ApiAuditEvent {
  audit_event_id: string
  team_id?: string
  actor_type: string
  actor_id: string
  action: string
  target_type?: string
  target_id?: string
  /** A short non-sensitive note — a role name, a model alias. */
  detail?: string
  created_at: number
}

export interface ApiAuditEventsResponse {
  events: ApiAuditEvent[]
  total: number
}

/** Upload response from the team-scoped upload endpoint. */
export interface UploadResponse {
  uploaded: string[]
}

/** Usage as returned by team usage endpoints (and the legacy personal alias). */
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
