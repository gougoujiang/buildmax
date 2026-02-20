/**
 * Map API DTOs to UI types and format values for display.
 * Imports API types from ./types and UI types from ../types.
 */

import type { ApiArtifact, ApiProject, ApiTask } from "./types"
import type { Artifact, Project, Task } from "../types"

/** Format a Unix timestamp (seconds) as "Today HH:MM", "Yesterday HH:MM", or full locale string. */
export function formatRelativeTime(secondsSinceEpoch: number): string {
  const d = new Date(secondsSinceEpoch * 1000)
  const today = new Date()
  if (d.toDateString() === today.toDateString()) {
    return `Today ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  const yesterday = new Date(today)
  yesterday.setDate(yesterday.getDate() - 1)
  if (d.toDateString() === yesterday.toDateString()) {
    return `Yesterday ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  }
  return d.toLocaleString()
}

function taskStatusToUI(status: string): Task["status"] {
  switch (status) {
    case "SUCCEEDED":
      return "success"
    case "FAILED":
      return "failed"
    case "CANCELED":
      return "canceled"
    case "PENDING":
      return "pending"
    case "RUNNING":
    default:
      return "running"
  }
}

export function apiArtifactToArtifact(api: ApiArtifact): Artifact {
  return {
    id: api.artifact_id,
    taskId: api.task_id,
    projectId: api.project_id ?? undefined,
    workspaceId: api.workspace_id,
    timeLabel: formatRelativeTime(api.created_at),
    title: api.task_input_snippet || `Artifact ${api.artifact_id}`,
  }
}

export function apiTaskToTask(api: ApiTask): Task {
  const title = api.input.length > 80 ? api.input.slice(0, 77) + "..." : api.input
  const summary =
    api.output ?? (api.input.length > 120 ? api.input.slice(0, 117) + "..." : api.input)
  const ts = api.ended_at ?? api.created_at
  return {
    id: api.id,
    projectId: api.project_id ?? undefined,
    sessionId: api.session_id ?? undefined,
    title,
    status: taskStatusToUI(api.status),
    timeLabel: formatRelativeTime(ts),
    summary,
  }
}

export function apiProjectToProject(api: ApiProject): Project {
  const created = new Date(api.created_at * 1000)
  const label =
    created.toDateString() === new Date().toDateString()
      ? "Created today"
      : `Created ${created.toLocaleDateString()}`
  return {
    id: api.id,
    workspaceId: api.workspace_id,
    name: api.name,
    status: "active",
    updatedAtLabel: label,
  }
}
