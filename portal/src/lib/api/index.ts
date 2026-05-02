/**
 * Foundational portal API exports.
 * Feature-owned API modules live under src/features/<feature>/api.ts.
 */

export { UNAUTHORIZED_EVENT, parseErrorResponse, getApiBase } from "./client"
export type {
  LoginUser,
  LoginResponse,
  ApiWorkspace,
  ApiAgent,
  ApiTask,
  ApiTasksListResponse,
  ApiSession,
  ApiSessionMessage,
  ApiUsage,
  CreateTaskRunResponse,
  ApiArtifact,
  ApiArtifactItem,
  UploadResponse,
  ApiConversation,
  ApiConversationsListResponse,
  ApiConversationMessage,
  ApiConversationMessagesResponse,
  CreateConversationResponse,
  AddConversationMessageResponse,
  ApiIssue,
  ApiIssuesListResponse,
  ApiWorkflow,
  ApiWorkflowListResponse,
  ApiWorkflowRun,
  ApiWorkflowRunListResponse,
  ApiWorkflowRunDetailResponse,
  ApiWorkflowStepRun,
} from "./types"
export {
  apiAgentToAgent,
  apiConversationToConversation,
  apiIssueToIssue,
  apiWorkflowToWorkflow,
  apiWorkflowRunToWorkflowRun,
  apiWorkflowStepRunToWorkflowStepRun,
} from "./mappers"
