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
  projectId: string
  title: string
  status: "running" | "success" | "failed" | "canceled"
  timeLabel: string
  summary: string
}

export interface Artifact {
  id: string
  projectId: string
  title: string
  kind: "report" | "chart" | "data" | "other"
  preview: string
}

// --- Route types ---

export type Route =
  | { name: "workspace"; workspaceId: string }
  | { name: "project"; workspaceId: string; projectId: string }
  | { name: "task"; workspaceId: string; projectId: string; taskId: string }
  | { name: "artifact"; workspaceId: string; projectId: string; artifactId: string }
