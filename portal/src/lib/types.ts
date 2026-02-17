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

// --- Route types ---

export type Route =
  | { name: "workspace"; workspaceId: string }
  | { name: "project"; workspaceId: string; projectId: string }
  | { name: "task"; workspaceId: string; projectId: string; taskId: string }
  | { name: "activity"; workspaceId: string }
  | { name: "explore"; workspaceId: string }

// --- Activity (workspace-scoped) ---

export interface ActivityItem {
  title: string
  time: string
}

// --- Explore (workspace directory structure) ---

export type ExploreNode =
  | { id: string; name: string; type: "folder"; children: ExploreNode[] }
  | { id: string; name: string; type: "file"; content?: string }
