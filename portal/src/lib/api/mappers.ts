/**
 * Map API DTOs to UI types and format values for display.
 * Imports API types from ./types and UI types from ../types.
 */

import type {
  ApiAgent,
  ApiArtifact,
  ApiTask,
  ApiConversation,
  ApiIssue,
  ApiWorkflow,
  ApiWorkflowRun,
  ApiWorkflowStepRun,
} from "./types"
import type {
  Agent,
  Artifact,
  Task,
  Conversation,
  Issue,
  Workflow,
  WorkflowRun,
  WorkflowStepRun,
} from "../types"

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

export function apiAgentToAgent(api: ApiAgent): Agent {
  return {
    id: api.id,
    name: api.name,
    description: api.description,
    instructions: api.instructions,
    createdAt: api.created_at,
  }
}

export function apiIssueToIssue(api: ApiIssue): Issue {
  return {
    id: api.id,
    userId: api.user_id,
    title: api.title,
    description: api.description,
    status: api.status as Issue["status"],
    assigneeKind: (api.assignee_kind as Issue["assigneeKind"]) ?? null,
    assigneeId: api.assignee_id ?? null,
    createdBy: api.created_by,
    createdAt: api.created_at,
    updatedAt: api.updated_at,
    updatedLabel: formatRelativeTime(api.updated_at),
  }
}

export function apiWorkflowToWorkflow(api: ApiWorkflow): Workflow {
  return {
    id: api.id,
    teamId: api.team_id,
    name: api.name,
    description: api.description,
    definition: api.definition,
    createdBy: api.created_by,
    createdAt: api.created_at,
    updatedAt: api.updated_at,
    updatedLabel: formatRelativeTime(api.updated_at),
  }
}

export function apiWorkflowRunToWorkflowRun(api: ApiWorkflowRun): WorkflowRun {
  return {
    id: api.id,
    workflowId: api.workflow_id,
    issueId: api.issue_id ?? null,
    conversationId: api.conversation_id,
    status: api.status as WorkflowRun["status"],
    createdBy: api.created_by,
    createdAt: api.created_at,
    startedAt: api.started_at ?? null,
    endedAt: api.ended_at ?? null,
    errorMessage: api.error_message ?? null,
    createdLabel: formatRelativeTime(api.created_at),
  }
}

export function apiWorkflowStepRunToWorkflowStepRun(api: ApiWorkflowStepRun): WorkflowStepRun {
  return {
    id: api.id,
    workflowRunId: api.workflow_run_id,
    stepId: api.step_id,
    stepIndex: api.step_index,
    stepType: api.step_type,
    targetAgentId: api.target_agent_id ?? null,
    prompt: api.prompt,
    status: api.status as WorkflowStepRun["status"],
    taskId: api.task_id ?? null,
    taskRunId: api.task_run_id ?? null,
    outputSummary: api.output_summary ?? null,
    errorMessage: api.error_message ?? null,
    createdAt: api.created_at,
    startedAt: api.started_at ?? null,
    endedAt: api.ended_at ?? null,
  }
}

export function apiArtifactToArtifact(api: ApiArtifact): Artifact {
  return {
    id: api.task_run_id,
    taskId: api.task_id,
    taskRunId: api.task_run_id,
    timeLabel: formatRelativeTime(api.created_at),
    title: api.task_input_snippet || `Run output ${api.task_run_id}`,
  }
}

export function apiTaskToTask(api: ApiTask): Task {
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
    status: taskStatusToUI(api.status),
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
