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

/** Workspace as returned by GET /api/workspaces */
export interface ApiWorkspace {
  id: string
  name: string
  owner_user_id?: string
  created_at?: number
}

/** Project as returned by GET/POST /api/workspaces/{id}/projects */
export interface ApiProject {
  id: string
  workspace_id: string
  name: string
  description: string
  created_at: number
}

/** Agent as returned by GET/POST /api/workspaces/{id}/agents */
export interface ApiAgent {
  id: string
  workspace_id: string
  name: string
  description: string
  instructions: string
  created_at: number
}

/** Paginated chats response when using limit/offset/executed_only (if backend supports it). */
export interface ApiChatsListResponse {
  chats: ApiChat[]
  total: number
}

/** Chat as returned by GET/POST /api/workspaces/{id}/chats */
export interface ApiChat {
  id: string
  workspace_id: string
  project_id: string | null
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
}

/** Conversation as returned by GET /api/workspaces/{id}/chats/{id}/conversation */
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

/** Response from POST /api/workspaces/{id}/chats/{chat_id}/runs */
export interface CreateChatRunResponse {
  chat_run_id: string
  chat_id: string
}

/** Artifact as returned by GET /api/workspaces/{id}/artifacts */
export interface ApiArtifact {
  artifact_id: string
  chat_id: string
  chat_run_id: string
  workspace_id: string
  project_id: string | null
  created_at: number
  seq: number
  chat_input_snippet: string
}

/** Artifact item as returned by GET /api/workspaces/{id}/artifacts/{id}/items */
export interface ApiArtifactItem {
  relative_path: string
}

/** Upload response from POST /api/workspaces/{id}/upload */
export interface UploadResponse {
  uploaded: string[]
}
