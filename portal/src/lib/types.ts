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

// --- Profile scope (derived from route) ---

export interface ProfileScope {
  profileId: string
  conversationId?: string
}

// --- Route types ---

export type Route =
  | { name: "home"; profileId: string }
  | { name: "newChat"; profileId: string }
  | { name: "conversation"; profileId: string; conversationId: string }
  | { name: "chats"; profileId: string }
  | { name: "explore"; profileId: string }
  | { name: "agents"; profileId: string }

// --- Agent (user-owned persona) ---

export interface Agent {
  id: string
  name: string
  description?: string
  instructions?: string
  createdAt: number
}

// --- Explore (user file structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
