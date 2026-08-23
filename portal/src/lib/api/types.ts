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
  created_at?: string
}

/** Agent as returned by team-scoped agent endpoints. */
export interface ApiAgent {
  id: string
  user_id: string
  team_id: string
  name: string
  description: string
  instructions: string
  /**
   * Catalog plugin names this agent loads for a background run. Nothing is
   * inherited from the team's activations: an agent that names none loads none.
   */
  plugins?: string[]
  revision: number
  created_at: string
}

export interface ApiAgentRevision {
  agent_id: string
  revision: number
  name: string
  description: string
  instructions: string
  created_by: string
  created_at: string
}

export interface ApiAgentRevisionListResponse {
  revisions: ApiAgentRevision[]
  total: number
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
  created_at: string
  updated_at: string
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
  created_at: string
  /** Absent until the comment is edited. */
  edited_at?: string | null
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
  revision: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface ApiWorkflowRevision {
  workflow_id: string
  revision: number
  name: string
  description: string
  definition: string
  status: string
  created_by: string
  created_at: string
}

export interface ApiWorkflowRevisionListResponse {
  revisions: ApiWorkflowRevision[]
  total: number
}

export interface ApiWorkflowListResponse {
  workflows: ApiWorkflow[]
}

export interface ApiWorkflowRun {
  id: string
  workflow_id: string
  workflow_revision?: number | null
  issue_id?: string | null
  conversation_id: string
  status: string
  created_by: string
  created_at: string
  started_at?: string | null
  ended_at?: string | null
  error_message?: string | null
}

export interface ApiWorkflowStepRun {
  id: string
  workflow_run_id: string
  step_id: string
  step_index: number
  step_type: string
  target_agent_id?: string | null
  agent_name?: string | null
  agent_description?: string | null
  agent_instructions?: string | null
  agent_revision?: number | null
  prompt: string
  status: string
  task_id?: string | null
  task_run_id?: string | null
  output_summary?: string | null
  error_message?: string | null
  created_at: string
  started_at?: string | null
  ended_at?: string | null
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
  /** Set when kind is "artifact": the whole address, no run needed. */
  artifact_id?: string
  filename?: string
  media_type?: string
  size_bytes?: number
  preview?: string
  preview_truncated: boolean
  source: ApiOutputSource
  created_at: string
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
  created_at: string
  started_at: string | null
  ended_at: string | null
  error_message: string | null
  agent_id?: string | null
  issue_id?: string | null
  /** The run behind the current status. Keys the trace and file routes. */
  last_run_id?: string | null
  /** Runs of this task that stored output files, newest first. */
  artifact_run_ids?: string[]
}

/** Where one task run came from, as returned by the run provenance endpoint. */
export interface ApiRunProvenance {
  task_run_id: string
  task_id: string
  status: string
  /** What the worker was given. Compare with source_message. */
  input: string
  created_by?: string
  created_by_type?: string
  trigger_source?: string
  retry_of_task_run_id?: string | null
  created_at: string
  source_message?: ApiRunSourceMessage | null
  agent?: ApiRunAgent | null
}

/** The agent definition a run executed under. */
export interface ApiRunAgent {
  id: string
  name?: string
  /** The revision the run was handed. 0 when it was not recorded. */
  revision?: number
  /** What the definition says now; ahead of revision means it was edited since. */
  current_revision?: number
  deleted?: boolean
}

/** What the person actually said, quoted for comparison with the run input. */
export interface ApiRunSourceMessage {
  id: string
  content: string
  truncated: boolean
  created_at: string
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

/**
 * Response from the team-scoped cancel endpoint.
 *
 * `cancel_requested` is the difference that matters to the UI: false means the
 * run is already over, true means it is still executing and its worker has been
 * asked to stop.
 */
export interface CancelTaskResponse {
  task_id: string
  task_run_id: string
  status: string
  cancel_requested: boolean
}

/**
 * The run a retry created, and the one it repeats.
 *
 * `task_run_id` is the new run; `retry_of_task_run_id` is the finished run whose
 * input it carries.
 */
export interface RetryTaskResponse {
  task_id: string
  task_run_id: string
  retry_of_task_run_id: string
  status: string
}

/** Run output (artifact) as returned by task/run artifact endpoints */
export interface ApiArtifact {
  id: string
  team_id: string
  filename: string
  media_type: string
  size_bytes: number
  sha256: string
  title?: string
  created_by_type: string
  created_by_id?: string
  source_type: string
  source_id?: string
  /** Whether the server will serve this content for display rather than download. */
  inline: boolean
  expires_at?: string
  created_at: string
}

export interface ApiArtifactList {
  items: ApiArtifact[]
  total: number
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
 * One managed model call a run made, as the governance ledger recorded it.
 *
 * This is a different record from the trace, and the difference matters when
 * reading a run: the trace is what the agent did, written by the run itself,
 * while this is what the deployment was asked to serve and account for. Only
 * calls that went through the managed gateway appear here — a deployment
 * running in direct mode records none, because the worker called the provider
 * itself and the server never saw it.
 *
 * It carries no prompts, tool payloads, or generated content, and omits the
 * catalog entry an alias resolved to: that is the operator's routing, not the
 * team's.
 */
export interface ApiTaskRunLLMCall {
  id: string
  user_id?: string
  task_id?: string
  surface?: string
  session_id?: string
  /** The operator-approved alias the run was allowed to call. */
  alias?: string
  streaming: boolean
  accepted_at: string
  first_delta_at?: string
  completed_at?: string
  status: string
  error_class?: string
  attempts?: number
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  /**
   * The cached parts of `prompt_tokens`, not tokens on top of it. A reader that
   * adds them to the prompt count counts the same tokens twice.
   */
  cache_read_tokens?: number
  cache_write_tokens?: number
  /**
   * Separates a provider that reported nothing from one that reported zero.
   * Without it an absent count reads as a free call.
   */
  usage_source?: string
  /**
   * What the call is estimated to have cost, priced at the rates recorded when
   * it ran rather than at whatever the catalog says now. Absent when the model
   * was unpriced or the provider reported no usage — an unpriced call is an
   * unknown, and a zero would read as a free one.
   */
  cost?: ApiLLMCallCost
}

/**
 * One call's estimated spend, in nano-units of `currency`: one currency unit is
 * 1e9 of them. Integers so a run sums exactly instead of accumulating float
 * error across hundreds of calls.
 *
 * `baseline` is what the same tokens would have cost with no caching at all.
 * It is the only honest way to say whether caching helped: comparing `total`
 * against zero would report a saving on a call that only ever wrote.
 */
export interface ApiLLMCallCost {
  currency: string
  uncached: number
  cache_read: number
  cache_write: number
  output: number
  total: number
  baseline: number
}

/**
 * One recorded action. The server records that something happened and who did
 * it — never prompts, generated content, tool output, or credentials.
 */
export interface ApiAuditEvent {
  id: string
  team_id?: string
  actor_type: string
  actor_id: string
  action: string
  target_type?: string
  target_id?: string
  /** A short non-sensitive note — a role name, a model alias. */
  detail?: string
  created_at: string
}

export interface ApiAuditEventsResponse {
  events: ApiAuditEvent[]
  total: number
}

// --- Deployment administration ---
//
// These come from /api/admin, which is deployment-scoped rather than
// team-scoped. Nothing here carries team content: an administrator learns that
// an account or a team exists, never what is in it.

/** One deployment-scoped grant. */
export interface ApiSystemGrant {
  id: string
  user_id: string
  role: string
  granted_by: string
  granted_at: string
  revoked_at?: string
  /** Resolved on the grants list so a reader does not see only ids. */
  email?: string
}

/** The caller's own deployment authority, from GET /api/admin/me. */
export interface ApiAdminMe {
  user_id: string
  roles: string[]
  grants: ApiSystemGrant[]
}

export interface ApiAdminGrantsResponse {
  grants: ApiSystemGrant[]
}

/** One account as an administrator sees it. Never a hash, never a token. */
export interface ApiAdminUser {
  id: string
  email: string
  name?: string
  quota_tier?: string
  has_password: boolean
  /** Non-null means every credential this account holds is refused. */
  disabled_at?: string
  last_login_at?: string
  last_login_platform?: string
  created_at: string
}

export interface ApiAdminUsersResponse {
  users: ApiAdminUser[]
  total: number
}

export interface ApiAdminUserTeam {
  team_id: string
  name: string
  role: string
}

export interface ApiAdminUserDetail extends ApiAdminUser {
  teams: ApiAdminUserTeam[]
  /** Live login chains, not tokens. */
  session_count: number
  system_roles: string[]
}

export interface ApiAdminLoginCode {
  code: string
  expires_at: string
}

export interface ApiAdminSessionsRevoked {
  revoked: number
}

/** An account plus what a disable did to its sessions. */
export interface ApiAdminUserAfterDisable extends ApiAdminUser {
  sessions_revoked: number
}

export interface ApiAdminDependency {
  name: string
  /** "ok" or "failed". The reason is deliberately not reported. */
  status: string
}

export interface ApiAdminSchemaMigration {
  id: string
  applied_at: string
}

export interface ApiAdminSystem {
  version: string
  schema_migrations: ApiAdminSchemaMigration[]
  dependencies: ApiAdminDependency[]
  ready: boolean
  worker_run_mode?: string
  worker_llm_transport?: string
  /** Empty when no worker path passes one, which is every deployment today. */
  sandbox_surface?: string
  allow_signup: boolean
  task_runs: Record<string, number>
  system_admins: number
  server_time: number
}

/**
 * One catalog model. There is no credential field, and there is none in the
 * server's record either — the key leaves the store only for the component that
 * opens a provider connection.
 */
export interface ApiAdminModel {
  id: string
  name: string
  provider_type: string
  api_url: string
  model: string
  context_window?: number
  call_timeout?: number
  max_tokens?: number
  reasoning?: string
  prompt_cache?: boolean
  vision?: boolean
  capabilities?: string[]
  enabled: boolean
  created_at: string
  updated_at: string
  /** Deployment aliases pointing here. None means no team can call it. */
  aliases: string[]
}

export interface ApiAdminModelsResponse {
  models: ApiAdminModel[]
  default_alias?: string
}

/** One catalog entry in the private plugin Marketplace. */
export interface ApiPlugin {
  name: string
  display_name?: string
  description?: string
  /** Non-zero means retired: out of the default catalog, and no new releases. */
  archived_at?: string
  created_by: string
  created_at: string
  updated_at: string
}

/**
 * What a release says it contributes.
 *
 * Names, transports, executables, and hosts — never arguments, header values,
 * environment values, prompts, or file contents. The server's inspection is
 * what decides that, and this shape can only carry what it kept.
 */
export interface ApiPluginInspection {
  skills?: string[]
  subagents?: { name: string; tools?: string[]; model?: string }[]
  mcp?: { id: string; transport: string; executable?: string; host?: string }[]
  hooks?: {
    event: string
    type: string
    matcher?: string
    executable?: string
    host?: string
    mcp_server?: string
    mcp_tool?: string
  }[]
  env_refs?: string[]
  plugin_paths?: string[]
  warnings?: string[]
}

/**
 * The publisher's claim about the checkout the bytes came from.
 *
 * Unlike the digest, the server cannot verify any of it, so it is shown as a
 * claim rather than as proof.
 */
/** Who fills a team's plugin activation list. */
export type ApiPluginCuration = "open" | "curated"

/** How an activation came to exist. */
export type ApiPluginActivationOrigin = "curated" | "automatic"

/**
 * One team's pinned use of one catalog plugin.
 *
 * The pin is the point: a release published after this row was written cannot
 * change what a run loads until somebody moves it.
 */
export interface ApiPluginActivation {
  id: string
  team_id: string
  plugin_name: string
  version: string
  digest: string
  enabled: boolean
  origin: ApiPluginActivationOrigin
  activated_by: string
  activated_at: string
  updated_by?: string
  updated_at: string
}

/**
 * What a team has activated, and who fills the list.
 *
 * The mode travels with the activations because reading one without the other
 * misleads: an empty list means "nothing activated yet" when open and "nothing
 * may be named" when curated.
 */
export interface ApiPluginActivationsResponse {
  curation: ApiPluginCuration
  activations: ApiPluginActivation[]
}

export interface ApiPluginReleaseSource {
  remote_url?: string
  commit?: string
  branch?: string
  dirty?: boolean
}

/** One immutable published version. */
export interface ApiPluginRelease {
  plugin_name: string
  version: string
  min_buildmax_version?: string
  digest: string
  object_key: string
  size_bytes: number
  inspection: ApiPluginInspection
  source: ApiPluginReleaseSource
  published_by: string
  published_at: string
  /** Non-zero means withdrawn from default selection. Nothing is deleted. */
  yanked_at?: string
  yanked_by?: string
  yanked_reason?: string
}

export interface ApiPluginsResponse {
  plugins: ApiPlugin[]
}

export interface ApiPluginReleasesResponse {
  releases: ApiPluginRelease[]
}

/** One entry and everything published under it. */
export interface ApiPluginResponse {
  plugin: ApiPlugin
  releases: ApiPluginRelease[]
}

/** Whether a credential is configured. Never its value, length, or prefix. */
export interface ApiSecretStatus {
  set: boolean
}

export interface ApiAdminTeam {
  id: string
  name: string
  personal: boolean
  quota_tier?: string
  member_count: number
  created_by?: string
  created_at: string
}

export interface ApiAdminTeamsResponse {
  teams: ApiAdminTeam[]
  total: number
}

export interface ApiAdminTeamMember {
  user_id: string
  email?: string
  role: string
}

export interface ApiAdminTeamDetail extends ApiAdminTeam {
  members: ApiAdminTeamMember[]
  usage?: ApiUsage
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
  created_at: string
  created_by: string
}

export interface ApiTeam {
  id: string
  name: string
  personal_for_user_id?: string | null
  created_at?: string
}

export interface ApiTeamMember {
  team_id: string
  user_id: string
  role: string
  created_at?: string
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
  created_at: string
}

/** Response from the team-scoped list conversation messages endpoint. */
export interface ApiConversationMessagesResponse {
  messages: ApiConversationMessage[]
}

/** Response from the team-scoped add conversation message endpoint. */
export interface AddConversationMessageResponse {
  reply: string
}
