// --- Entity types ---

export interface Workspace {
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
  workspaceId: string
  timeLabel: string
  title: string
}

// --- Workspace scope (derived from route) ---
// Scope = what is in context for the current view (workspaceId; chatId when on chat).
// Route = URL state; scope = derived context for data and display.

export interface WorkspaceScope {
  workspaceId: string
  chatId?: string
}

// --- Route types ---
// Portal uses chat/chats; one chat = backend chat.

export type Route =
  | { name: "workspace"; workspaceId: string }
  | { name: "newChat"; workspaceId: string }
  | { name: "chat"; workspaceId: string; chatId: string }
  | { name: "chats"; workspaceId: string }
  | { name: "explore"; workspaceId: string }
  | { name: "agents"; workspaceId: string }

// --- View run output (artifact modal) ---

export interface ViewArtifactParams {
  workspaceId: string
  chatRunId: string
}

// --- Agent (workspace-scoped persona) ---

export interface Agent {
  id: string
  workspaceId: string
  name: string
  description?: string
  instructions?: string
  createdAt: number
}

// --- Explore (workspace directory structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
