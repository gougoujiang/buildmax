import {
  checkUnauthorized,
  getApiBase,
  parseErrorResponse,
  requestJson,
} from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import { readSSEStream } from "../../lib/api/sse"
import type {
  AddConversationMessageResponse,
  ApiConversationMessagesResponse,
  ApiConversationsListResponse,
  CreateConversationResponse,
} from "../../lib/api/types"

export interface ConversationStreamCallbacks {
  onConversationId?: (id: string) => void
  onDelta: (text: string) => void
  onDone: () => void
  onError: (err: Error) => void
}

function tryParseConversationStreamEvent(
  data: string
): { conversationId?: string; error?: string } {
  try {
    const obj = JSON.parse(data) as Record<string, unknown>
    if (typeof obj.conversation_id === "string") return { conversationId: obj.conversation_id }
    if (typeof obj.error === "string") return { error: obj.error }
  } catch {
    /* not json */
  }
  return {}
}

export async function getConversations(
  _profileId: string,
  token: string,
  options?: { limit?: number; offset?: number }
): Promise<ApiConversationsListResponse> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  const q = params.toString()
  const url = `${getApiBase()}/api/conversations${q ? `?${q}` : ""}`
  return requestJson<ApiConversationsListResponse>(url, { headers: authHeaders(token) })
}

export async function createConversation(
  _profileId: string,
  body: { channel?: string; message?: string },
  token: string
): Promise<CreateConversationResponse> {
  return requestJson<CreateConversationResponse>(`${getApiBase()}/api/conversations`, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export async function createConversationStream(
  _profileId: string,
  body: { channel?: string; message?: string },
  token: string,
  callbacks: ConversationStreamCallbacks
): Promise<void> {
  const url = `${getApiBase()}/api/conversations?stream=1`
  const res = await fetch(url, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
  })
  checkUnauthorized(res)
  if (!res.ok) {
    const msg = await parseErrorResponse(res, "Create conversation failed")
    callbacks.onError(new Error(msg))
    return
  }

  await readSSEStream(res, {
    onData: (data) => {
      if (data === "done") {
        callbacks.onDone()
        return false
      }
      const parsed = tryParseConversationStreamEvent(data)
      if (parsed.conversationId !== undefined) {
        callbacks.onConversationId?.(parsed.conversationId)
        return
      }
      if (parsed.error !== undefined) {
        callbacks.onError(new Error(parsed.error))
        return false
      }
      callbacks.onDelta(data)
    },
    onDone: callbacks.onDone,
    onError: callbacks.onError,
  })
}

export async function addConversationMessageStream(
  _profileId: string,
  conversationId: string,
  body: { content: string },
  token: string,
  callbacks: Pick<ConversationStreamCallbacks, "onDelta" | "onDone" | "onError">,
  options?: { signal?: AbortSignal }
): Promise<void> {
  const url = `${getApiBase()}/api/conversations/${encodeURIComponent(conversationId)}/messages?stream=1`
  const res = await fetch(url, {
    method: "POST",
    headers: { ...jsonHeaders, ...authHeaders(token) },
    body: JSON.stringify(body),
    signal: options?.signal,
  })
  checkUnauthorized(res)
  if (!res.ok) {
    const msg = await parseErrorResponse(res, "Send message failed")
    callbacks.onError(new Error(msg))
    return
  }

  await readSSEStream(res, {
    onData: (data) => {
      if (data === "done") {
        callbacks.onDone()
        return false
      }
      const parsed = tryParseConversationStreamEvent(data)
      if (parsed.error !== undefined) {
        callbacks.onError(new Error(parsed.error))
        return false
      }
      callbacks.onDelta(data)
    },
    onDone: callbacks.onDone,
    onError: callbacks.onError,
  })
}

export async function getConversationMessages(
  _profileId: string,
  conversationId: string,
  token: string
): Promise<ApiConversationMessagesResponse> {
  return requestJson<ApiConversationMessagesResponse>(
    `${getApiBase()}/api/conversations/${encodeURIComponent(conversationId)}/messages`,
    { headers: authHeaders(token) }
  )
}

export async function addConversationMessage(
  _profileId: string,
  conversationId: string,
  body: { content: string },
  token: string
): Promise<AddConversationMessageResponse> {
  return requestJson<AddConversationMessageResponse>(
    `${getApiBase()}/api/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      method: "POST",
      headers: { ...jsonHeaders, ...authHeaders(token) },
      body: JSON.stringify(body),
    }
  )
}
