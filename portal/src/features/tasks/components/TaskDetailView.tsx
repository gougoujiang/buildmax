import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer, ChatThread, type ChatThreadItem } from "@buildmax/gui"
import { AgentAvatar, UserAvatar } from "../../../components/UserAvatar"
import type { LoginUser } from "../../../lib/api"
import type { ApiSession } from "../../../lib/api"

interface TaskDetailViewProps {
  historyRef: React.RefObject<HTMLElement | null>
  session: ApiSession | null
  sessionLoading: boolean
  sessionError: string | null
  followUpInput: string
  setFollowUpInput: (value: string) => void
  followUpLoading: boolean
  followUpError: string | null
  streamingContent: string
  lastSentMessage: string | null
  user: LoginUser | null
  initialInput?: string
  showInitialInput: boolean
  expandedToolIndices: Set<number>
  toggleToolExpand: (index: number) => void
  onSubmitFollowUp: () => void
}

export function TaskDetailView({
  historyRef,
  session,
  sessionLoading,
  sessionError,
  followUpInput,
  setFollowUpInput,
  followUpLoading,
  followUpError,
  streamingContent,
  lastSentMessage,
  user,
  initialInput,
  showInitialInput,
  expandedToolIndices,
  toggleToolExpand,
  onSubmitFollowUp,
}: TaskDetailViewProps) {
  const items: ChatThreadItem[] = []

  if (showInitialInput && initialInput) {
    items.push({
      id: "initial-input",
      role: "user",
      label: "You",
      avatar: user ? <UserAvatar user={user} size="sm" /> : <AgentAvatar size="sm" />,
      body: (
        <div className="page-chat__msg-content page-chat__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{initialInput}</Markdown>
        </div>
      ),
    })
  }

  if (session && !sessionLoading && !sessionError) {
    session.messages.forEach((msg, i) => {
      const isUser = msg.role === "user"
      const isTool = msg.role === "tool"
      const isToolExpanded = expandedToolIndices.has(i)

      items.push({
        id: `session-msg-${i}`,
        role: msg.role,
        label: isUser ? "You" : msg.role,
        hideAvatar: isTool,
        avatar: !isTool
          ? isUser && user
            ? <UserAvatar user={user} size="sm" />
            : <AgentAvatar size="sm" />
          : undefined,
        collapsed: isTool && !isToolExpanded,
        body: isTool ? (
          <>
            <button
              type="button"
              className="page-chat__tool-toggle"
              onClick={() => toggleToolExpand(i)}
              aria-expanded={isToolExpanded}
              aria-controls={`tool-content-${i}`}
              id={`tool-toggle-${i}`}
            >
              <span className="page-chat__tool-toggle-label">Tool result</span>
              <span className="page-chat__tool-chevron" aria-hidden>
                {isToolExpanded ? "▲" : "▼"}
              </span>
            </button>
            <div
              id={`tool-content-${i}`}
              className="page-chat__tool-content"
              hidden={!isToolExpanded}
              role="region"
              aria-labelledby={`tool-toggle-${i}`}
            >
              {msg.content ? (
                <div className="page-chat__msg-content page-chat__markdown">
                  <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
                </div>
              ) : null}
            </div>
          </>
        ) : (
          <>
            {msg.content ? (
              <div className="page-chat__msg-content page-chat__markdown">
                <Markdown remarkPlugins={[remarkGfm]}>{msg.content}</Markdown>
              </div>
            ) : null}
            {msg.tool_calls && msg.tool_calls.length > 0 ? (
              <ul className="page-chat__msg-toolcalls">
                {msg.tool_calls.map((tc) => (
                  <li key={tc.id}>
                    <strong>{tc.name}</strong>
                    {tc.arguments ? (
                      <pre className="page-chat__msg-args">{tc.arguments}</pre>
                    ) : null}
                  </li>
                ))}
              </ul>
            ) : null}
          </>
        ),
      })
    })
  }

  if (lastSentMessage) {
    items.push({
      id: "last-sent-message",
      role: "user",
      label: "You",
      avatar: user ? <UserAvatar user={user} size="sm" /> : <AgentAvatar size="sm" />,
      body: (
        <div className="page-chat__msg-content page-chat__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{lastSentMessage}</Markdown>
        </div>
      ),
    })
  }

  if (streamingContent) {
    items.push({
      id: "streaming-assistant",
      role: "assistant",
      label: "Assistant (streaming)",
      avatar: <AgentAvatar size="sm" />,
      body: (
        <div className="page-chat__msg-content page-chat__markdown">
          <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
        </div>
      ),
    })
  }

  return (
    <div className="page-chat">
      <ChatThread
        historyRef={historyRef}
        ariaLabel="Task conversation history"
        items={items}
        loadingText={sessionLoading ? "Loading conversation…" : null}
        errorText={sessionError}
        emptyText={!initialInput ? "No messages yet. Use the input below to start." : null}
      />

      <section className="page-chat__input">
        <ChatComposer
          value={followUpInput}
          onChange={setFollowUpInput}
          onSubmit={onSubmitFollowUp}
          loading={followUpLoading}
          error={followUpError}
          placeholder="Ask a follow-up… (Enter to send, Shift+Enter for new line)"
          ariaLabel="Task follow-up input"
        />
      </section>
    </div>
  )
}
