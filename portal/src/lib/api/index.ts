/**
 * Compatibility barrel for portal API imports.
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
} from "./types"
export {
  apiAgentToAgent,
  apiArtifactToArtifact,
  apiTaskToTask,
  apiConversationToConversation,
} from "./mappers"

export { requestOtp, login } from "../../features/auth"
export { getUsage } from "../../features/usage"
export { getAgents, createAgent, updateAgent, deleteAgent } from "../../features/agents"
export {
  getTasks,
  getTasksPaginated,
  getTaskConversation,
  createTask,
  createTaskRun,
  subscribeTaskStream,
} from "../../features/tasks"
export type {
  CreateTaskBody,
  GetTasksPaginatedOptions,
  RunStreamCallbacks,
} from "../../features/tasks"
export {
  getConversations,
  createConversation,
  createConversationStream,
  getConversationMessages,
  addConversationMessage,
  addConversationMessageStream,
} from "../../features/conversations"
export type { ConversationStreamCallbacks } from "../../features/conversations"
export { getArtifacts, getArtifactItems, getArtifactContent } from "../../features/artifacts"
export { uploadFiles, getFileTree, getFileContent } from "../../features/files"
