// --- Entity types ---

export interface Profile {
  id: string
  name: string
}

/** Background task (Tier 2) created from a conversation. Backend: Task. */
export interface Task {
  id: string
  sessionId?: string
  title: string
  status: "pending" | "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
  /** Set when the task was started from an agent. */
  agentId?: string
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
  assigneeKind?: "person" | "agent" | null
  assigneeId?: string | null
  createdBy: string
  createdAt: number
  updatedAt: number
  updatedLabel: string
}

// --- Explore (user file structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
