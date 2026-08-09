import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer, ChatThread, type ChatThreadItem } from "@buildmax/gui"
import { AgentAvatar, UserAvatar } from "../../../components/UserAvatar"
import type { ApiConversationMessage, LoginUser } from "../../../lib/api"

interface ConversationDetailViewProps {
  historyRef: React.RefObject<HTMLElement | null>
  messages: ApiConversationMessage[]
  messagesLoading: boolean
  messagesError: string | null
  input: string
  setInput: (value: string) => void
  sending: boolean
  sendError: string | null
  streamingContent: string | null
  /** Shown after “new chat” navigation until the first user message appears from the server. */
  optimisticUserMessage: string | null
  user: LoginUser | null
  onSend: () => void
}

export function ConversationDetailView({
  historyRef,
  messages,
  messagesLoading,
  messagesError,
  input,
  setInput,
  sending,
  sendError,
  streamingContent,
  optimisticUserMessage,
  user,
  onSend,
}: ConversationDetailViewProps) {
  const items: ChatThreadItem[] = messages.map((msg) => {
    const isUser = msg.role === "user"

    return {
      id: msg.id,
      role: msg.role,
      label: isUser ? "You" : msg.role,
      avatar: isUser && user ? <UserAvatar user={user} size="sm" /> : <AgentAvatar size="sm" />,
      body: msg.content ? (
        <div className="page-chat__msg-content page-chat__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
        </div>
      ) : null,
    }
  })

  const showOptimisticUser =
    optimisticUserMessage &&
    !messages.some(
      (m) => m.role === "user" && m.content === optimisticUserMessage
    )

  if (showOptimisticUser) {
    items.push({
      id: "optimistic-user",
      role: "user",
      label: "You",
      avatar: user ? <UserAvatar user={user} size="sm" /> : undefined,
      body: (
        <div className="page-chat__msg-content page-chat__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{optimisticUserMessage}</Markdown>
        </div>
      ),
    })
  }

  if (streamingContent !== null) {
    items.push({
      id: "streaming-assistant",
      role: "assistant",
      label: "Assistant (streaming)",
      avatar: <AgentAvatar size="sm" />,
      body: (
        <div className="page-chat__msg-content page-chat__markdown">
          {streamingContent ? (
            <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
          ) : (
            <p className="bm-chat-thread__text bm-chat-thread__text--muted">Thinking…</p>
          )}
        </div>
      ),
    })
  }

  return (
    <div className="page-chat">
      <ChatThread
        historyRef={historyRef}
        ariaLabel="Conversation history"
        items={items}
        loadingText={messagesLoading ? "Loading conversation…" : null}
        errorText={messagesError}
        emptyText="No messages yet. Use the input below to start."
      />

      <section className="page-chat__input" aria-label="Send a message">
        <ChatComposer
          value={input}
          onChange={setInput}
          onSubmit={onSend}
          loading={sending}
          error={sendError}
          placeholder="Type a message… (Enter to send, Shift+Enter for new line)"
          ariaLabel="Message"
        />
      </section>
    </div>
  )
}
