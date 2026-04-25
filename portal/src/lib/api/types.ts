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

/** Agent as returned by GET/POST /api/agents */
export interface ApiAgent {
  id: string
  user_id: string
  name: string
  description: string
  instructions: string
  created_at: number
}

export interface ApiIssue {
  id: string
  user_id: string
  title: string
  description: string
  status: string
  assignee_kind?: string | null
  assignee_id?: string | null
  created_by: string
  created_at: number
  updated_at: number
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

/** Task as returned by task endpoints (GET /api/tasks/{id}, POST /api/conversations/{id}/tasks). */
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
}

/** Conversation as returned by GET /api/tasks/{id}/conversation */
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

/** Response from POST /api/tasks/{task_id}/runs */
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

/** Artifact item as returned by GET /api/task-runs/{task_run_id}/artifacts/items */
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

/** Tier 1 conversation as returned by GET /api/conversations */
export interface ApiConversation {
  id: string
  user_id: string
  channel: string
  title?: string
  created_at: number
  created_by: string
}

/** Response from GET /api/conversations */
export interface ApiConversationsListResponse {
  conversations: ApiConversation[]
  total: number
}

/** Response from POST /api/conversations */
export interface CreateConversationResponse {
  conversation_id: string
  reply?: string
}

/** Message as returned by GET /api/conversations/{id}/messages */
export interface ApiConversationMessage {
  id: string
  role: string
  content: string
  channel?: string | null
  created_at: number
}

/** Response from GET /api/conversations/{id}/messages */
export interface ApiConversationMessagesResponse {
  messages: ApiConversationMessage[]
}

/** Response from POST /api/conversations/{id}/messages */
export interface AddConversationMessageResponse {
  reply: string
}
