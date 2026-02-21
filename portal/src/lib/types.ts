// --- Entity types ---

export interface Workspace {
  id: string
  name: string
}

export interface Project {
  id: string
  workspaceId: string
  name: string
  status: "active" | "paused" | "archived"
  updatedAtLabel: string
}

export interface Task {
  id: string
  projectId?: string
  sessionId?: string
  title: string
  status: "pending" | "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
}

export interface Artifact {
  id: string
  taskId: string
  projectId?: string
  workspaceId: string
  timeLabel: string
  title: string
}

// --- Workspace scope (derived from route) ---
// Scope = what is in context for the current view (workspaceId; projectId/taskId when on project/task).
// Route = URL state; scope = derived context for data and display.

export interface WorkspaceScope {
  workspaceId: string
  projectId?: string
  taskId?: string
}

// --- Route types ---

export type Route =
  | { name: "workspace"; workspaceId: string }
  | { name: "project"; workspaceId: string; projectId: string }
  | { name: "task"; workspaceId: string; projectId: string; taskId: string }
  | { name: "activity"; workspaceId: string }
  | { name: "explore"; workspaceId: string }
  | { name: "agents"; workspaceId: string }
  | { name: "agentList"; workspaceId: string }

// --- Activity (workspace-scoped) ---

export interface ActivityItem {
  title: string
  time: string
}

// --- View artifact (modal) ---

export interface ViewArtifactParams {
  workspaceId: string
  artifactId: string
}

// --- Agent (workspace-scoped persona) ---

export interface Agent {
  id: string
  workspaceId: string
  name: string
  description: string
  instructions: string
  createdAt: number
}

// --- Explore (workspace directory structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
