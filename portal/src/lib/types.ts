// --- Entity types ---

export interface Profile {
  id: string
  name: string
}

export interface Chat {
  id: string
  sessionId?: string
  title: string
  status: "pending" | "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
  /** Set when the chat was started from an agent. */
  agentId?: string
}

export interface Artifact {
  id: string
  chatId: string
  chatRunId?: string
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
// Scope = what is in context for the current view (profileId; chatId when on chat).
// Route = URL state; scope = derived context for data and display.

export interface ProfileScope {
  profileId: string
  chatId?: string
  conversationId?: string
}

// --- Route types ---
// Portal uses chat/chats; one chat = backend chat.

export type Route =
  | { name: "home"; profileId: string }
  | { name: "newChat"; profileId: string }
  | { name: "chat"; profileId: string; chatId: string }
  | { name: "conversation"; profileId: string; conversationId: string }
  | { name: "chats"; profileId: string }
  | { name: "explore"; profileId: string }
  | { name: "agents"; profileId: string }

// --- View run output (artifact modal) ---

export interface ViewArtifactParams {
  chatRunId: string
}

// --- Agent (user-owned persona) ---

export interface Agent {
  id: string
  name: string
  description?: string
  instructions?: string
  createdAt: number
}

// --- Explore (workspace directory structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
