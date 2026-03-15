/**
 * Map API DTOs to UI types and format values for display.
 * Imports API types from ./types and UI types from ../types.
 */

import type { ApiAgent, ApiArtifact, ApiChat, ApiConversation } from "./types"
import type { Agent, Artifact, Chat, Conversation } from "../types"

/** Format a Unix timestamp (seconds) as "Today HH:MM", "Yesterday HH:MM", or full locale string. */
function formatRelativeTime(secondsSinceEpoch: number): string {
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

function chatStatusToUI(status: string): Chat["status"] {
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

export function apiAgentToAgent(api: ApiAgent): Agent {
  return {
    id: api.id,
    workspaceId: api.workspace_id,
    name: api.name,
    description: api.description,
    instructions: api.instructions,
    createdAt: api.created_at,
  }
}

export function apiArtifactToArtifact(api: ApiArtifact): Artifact {
  return {
    id: api.task_run_id,
    chatId: api.task_id,
    chatRunId: api.task_run_id,
    workspaceId: api.workspace_id,
    timeLabel: formatRelativeTime(api.created_at),
    title: api.task_input_snippet || `Run output ${api.task_run_id}`,
  }
}

export function apiChatToChat(api: ApiChat): Chat {
  const title =
    api.title && api.title.trim() !== ""
      ? api.title
      : api.input.length > 80
        ? api.input.slice(0, 77) + "..."
        : api.input
  const summary =
    api.output ?? (api.input.length > 120 ? api.input.slice(0, 117) + "..." : api.input)
  const ts = api.ended_at ?? api.created_at
  return {
    id: api.id,
    sessionId: api.session_id ?? undefined,
    title,
    status: chatStatusToUI(api.status),
    timeLabel: formatRelativeTime(ts),
    summary,
    agentId: api.agent_id ?? undefined,
  }
}

export function apiConversationToConversation(api: ApiConversation): Conversation {
  return {
    id: api.id,
    channel: api.channel,
    title: api.title?.trim() ?? "",
    created_at: api.created_at,
    timeLabel: formatRelativeTime(api.created_at),
  }
}
