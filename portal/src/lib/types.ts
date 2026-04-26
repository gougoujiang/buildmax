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
  createdAt: number
  /** Set when the task was started from an agent. */
  agentId?: string
  /** Set when the task was started from an issue. */
  issueId?: string
}

export interface Artifact {
  id: string
  taskId: string
  taskRunId?: string
  timeLabel: string
  title: string
}

/** Tier 1 conversation (user-facing dialogue). */
export interface Conversation {
  id: string
  channel: string
  title: string
  created_at: number
  timeLabel: string
}

// --- Route types ---

export type Route =
  | { name: "home" }
  | { name: "login" }
  | { name: "signup" }
  | { name: "conversation"; conversationId: string }
  | { name: "chats" }
  | { name: "explore" }
  | { name: "agents" }
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
  createdAt: number
}

export interface Issue {
  id: string
  userId: string
  title: string
  description: string
  status: "todo" | "in_progress" | "done"
  assigneeKind?: "person" | "agent" | "workflow" | null
  assigneeId?: string | null
  createdBy: string
  createdAt: number
  updatedAt: number
  updatedLabel: string
}

export interface Workflow {
  id: string
  teamId: string
  name: string
  description: string
  definition: string
  createdBy: string
  createdAt: number
  updatedAt: number
  updatedLabel: string
}

export interface WorkflowRun {
  id: string
  workflowId: string
  issueId?: string | null
  conversationId: string
  status: "pending" | "running" | "succeeded" | "failed" | "canceled"
  createdBy: string
  createdAt: number
  startedAt?: number | null
  endedAt?: number | null
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
  prompt: string
  status: "pending" | "running" | "succeeded" | "failed" | "blocked"
  taskId?: string | null
  taskRunId?: string | null
  outputSummary?: string | null
  errorMessage?: string | null
  createdAt: number
  startedAt?: number | null
  endedAt?: number | null
}

export interface IssueFlowRun {
  run: WorkflowRun
  steps: WorkflowStepRun[]
}

export interface IssueFlow {
  issue: Issue
  workflow?: Workflow | null
  runs: IssueFlowRun[]
  agentTasks: Task[]
  total: number
}

// --- Explore (user file structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
