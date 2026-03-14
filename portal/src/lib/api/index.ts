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
  ApiChat,
  ApiChatsListResponse,
  ApiSession,
  ApiSessionMessage,
  ApiUsage,
  CreateChatRunResponse,
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
  apiChatToChat,
  apiConversationToConversation,
} from "./mappers"

export { requestOtp, login } from "../../features/auth"
export { getUsage } from "../../features/usage"
export { createWorkspace, getWorkspaces } from "../../features/workspaces"
export { getAgents, createAgent, updateAgent, deleteAgent } from "../../features/agents"
export {
  getChats,
  getChatsPaginated,
  getChatConversation,
  createChat,
  createChatRun,
  subscribeChatStream,
} from "../../features/chats"
export type {
  CreateChatBody,
  GetChatsPaginatedOptions,
  RunStreamCallbacks,
} from "../../features/chats"
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
