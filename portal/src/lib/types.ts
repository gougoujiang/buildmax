// --- Entity types ---

export interface Profile {
  id: string
  name: string
}

/** Background task (Tier 2) created from a conversation. Backend: Task. */
export interface Task {
  id: string
  conversationId: string
  sessionId?: string
  title: string
  status: "pending" | "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
  createdAt: string
  /** Set when the task was started from an agent. */
  agentId?: string
  /** Set when the task was started from an issue. */
  issueId?: string
}

/** Tier 1 conversation (user-facing dialogue). */
export interface Conversation {
  id: string
  channel: string
  title: string
  createdAt: string
  timeLabel: string
}

// --- Route types ---

export type Route =
  | { name: "home" }
  | { name: "login" }
  | { name: "conversation"; conversationId: string }
  | { name: "conversations" }
  | { name: "explore" }
  | { name: "agents" }
  | {
      name: "account"
      section?: "general" | "usage" | "webhook" | "plugins" | "invitations"
    }
  | {
      name: "space"
      section?: "overview" | "members" | "artifacts" | "plugins" | "secrets" | "audit" | "memberNew"
    }
  | { name: "admin"; section?: "overview" | "accounts" | "teams" | "models" | "plugins" | "audit" }
  | { name: "workflows" }
  | { name: "workflow"; workflowId: string }
  | { name: "workflowRun"; workflowRunId: string }
  | { name: "issues" }
  | { name: "issue"; issueId: string }

// --- Agent (user-owned persona) ---

export interface Agent {
  id: string
  name: string
  description?: string
  instructions?: string
  sandboxNetworkTier?: string
  sandboxFilesystemTier?: string
  secretConsumption?: import("./api/types").ApiSecretConsumption
  revision: number
  createdAt: string
}

export interface AgentRevision {
  id: string
  agentId: string
  revision: number
  name: string
  description: string
  instructions: string
  createdBy: string
  createdAt: string
  createdLabel: string
}

export interface Issue {
  id: string
  userId: string
  parentIssueId?: string | null
  title: string
  description: string
  status: "todo" | "in_progress" | "done"
  assigneeKind?: "person" | "agent" | "workflow" | null
  assigneeId?: string | null
  createdBy: string
  createdAt: string
  updatedAt: string
  updatedLabel: string
  /** Optimistic-concurrency token; an update must send the version it read. */
  version: number
  /** Derived server-side per response, never stored. */
  childCount: number
  doneChildCount: number
  commentCount: number
}

export interface Workflow {
  id: string
  teamId: string
  name: string
  description: string
  definition: string
  status: "draft" | "published" | "archived"
  revision: number
  createdBy: string
  createdAt: string
  updatedAt: string
  updatedLabel: string
}

export interface WorkflowRevision {
  id: string
  workflowId: string
  revision: number
  name: string
  description: string
  definition: string
  status: string
  createdBy: string
  createdAt: string
  createdLabel: string
}

export interface WorkflowRun {
  id: string
  workflowId: string
  workflowRevision?: number | null
  issueId?: string | null
  conversationId: string
  status: "pending" | "running" | "succeeded" | "failed" | "canceled"
  createdBy: string
  createdAt: string
  startedAt?: string | null
  endedAt?: string | null
  errorMessage?: string | null
  createdLabel: string
}

export interface WorkflowStepRun {
  id: string
  workflowRunId: string
  stepId: string
  stepIndex: number
  stepType: string
  targetAgentId?: string | null
  agentRevision?: number | null
  agentName?: string | null
  agentDescription?: string | null
  agentInstructions?: string | null
  prompt: string
  status: "pending" | "running" | "succeeded" | "failed" | "canceled" | "blocked"
  taskId?: string | null
  taskRunId?: string | null
  outputSummary?: string | null
  errorMessage?: string | null
  createdAt: string
  startedAt?: string | null
  endedAt?: string | null
}

export interface IssueFlowRun {
  run: WorkflowRun
  steps: WorkflowStepRun[]
}

export interface OutputSource {
  sourceType: string
  taskId?: string
  taskRunId?: string
  conversationId?: string
  workflowRunId?: string | null
  workflowStepRunId?: string | null
  workflowStepId?: string | null
}

export interface IssueOutput {
  id: string
  title: string
  kind: string
  relativePath?: string
  /** Set when kind is "artifact": the whole address, no run needed. */
  artifactId?: string
  filename?: string
  mediaType?: string
  sizeBytes?: number
  preview?: string
  previewTruncated: boolean
  source: OutputSource
  createdAt: string
}

export interface IssueFlow {
  issue: Issue
  /** Set on a sub-issue; children is set on a parent. Never both. */
  parent: Issue | null
  children: Issue[]
  workflow?: Workflow | null
  runs: IssueFlowRun[]
  agentTasks: Task[]
  latestResult: IssueOutput | null
  outputs: IssueOutput[]
  total: number
}

// --- Explore (user file structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
