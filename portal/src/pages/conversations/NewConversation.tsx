import { useState } from "react"
import { ChatComposer } from "@buildmax/gui"
import { navigate } from "../../router"
import { getErrorMessage } from "../../lib/errorMessage"
import { cn } from "../../lib/cn"
import { createConversation } from "../../features/conversations"
import { useApp } from "../../contexts/AppContext"
import { Alert } from "../../components/state/Alert"
import { EmptyState } from "../../components/state/EmptyState"
import type { ResourceState } from "../../state/resourceState"
import type { Conversation } from "../../lib/types"

type NewConversationTab = "conversations" | "files"

interface NewConversationProps {
  token?: string
  spaceId: string
  onRefetchConversations?: () => void
  conversations: Conversation[]
  conversationsState: ResourceState<Conversation[]>
}

export function NewConversation({
  token,
  spaceId,
  onRefetchConversations,
  conversations,
  conversationsState,
}: NewConversationProps) {
  const { setPendingConversation } = useApp()
  const [prompt, setPrompt] = useState("")
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<NewConversationTab>("conversations")

  async function handleSend() {
    const input = prompt.trim()
    if (!input || !token || !spaceId || running) return
    setRunning(true)
    setRunError(null)
    try {
      const created = await createConversation(spaceId, { channel: "portal" }, token)
      setPendingConversation({
        conversationId: created.conversation_id,
        initialMessage: input,
      })
      onRefetchConversations?.()
      setPrompt("")
      navigate({
        name: "chat",
        spaceId,
        conversationId: created.conversation_id,
      })
    } catch (err) {
      setRunError(getErrorMessage(err, "Failed to start conversation"))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="page-new-chat">
      <p className="page-new-chat__subtitle">
        Start a new conversation. Describe what you want to accomplish and the agent will work on it.
      </p>
      <section className="page-chat__input">
        <ChatComposer
          value={prompt}
          onChange={(value) => {
            setPrompt(value)
            setRunError(null)
          }}
          onSubmit={handleSend}
          loading={running}
          error={runError}
          placeholder="e.g. Help me analyze last month's sales data (Enter to send, Shift+Enter for new line)"
          ariaLabel="What would you like to do?"
        />
      </section>

      <div className="page-new-chat__tabs">
        <div className="page-new-chat__tab-list" role="tablist" aria-label="Recent conversations and files">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "conversations"}
            aria-controls="new-chat-tabpanel-conversations"
            id="new-chat-tab-conversations"
            className={cn("page-new-chat__tab", activeTab === "conversations" && "page-new-chat__tab--active")}
            onClick={() => setActiveTab("conversations")}
          >
            Recent Conversations
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "files"}
            aria-controls="new-chat-tabpanel-files"
            id="new-chat-tab-files"
            className={cn("page-new-chat__tab", activeTab === "files" && "page-new-chat__tab--active")}
            onClick={() => setActiveTab("files")}
          >
            Files
          </button>
        </div>

        <div
          id="new-chat-tabpanel-conversations"
          role="tabpanel"
          aria-labelledby="new-chat-tab-conversations"
          hidden={activeTab !== "conversations"}
          className="page-new-chat__tabpanel"
        >
          {activeTab === "conversations" && (
            <div className="page-new-chat__chats">
              {(conversationsState.kind === "error" ||
                conversationsState.kind === "forbidden" ||
                conversationsState.kind === "notFound" ||
                conversationsState.kind === "stale") && (
                <Alert
                  tone={conversationsState.kind === "stale" ? "stale" : conversationsState.kind}
                  message={conversationsState.error.message}
                  retry={{ label: "Retry", onClick: () => onRefetchConversations?.() }}
                />
              )}
              {conversationsState.kind === "loading" ? (
                <p className="page-activity__empty">Loading…</p>
              ) : conversationsState.kind === "readyEmpty" ? (
                <EmptyState message="No conversations yet. Send a message above to start one." />
              ) : conversationsState.kind === "error" ||
                conversationsState.kind === "forbidden" ||
                conversationsState.kind === "notFound" ? null : (
                <ul className="page-activity__list">
                  {conversations.map((conv) => (
                    <li key={conv.id} className="page-activity__item">
                      <button
                        type="button"
                        className="page-activity__link"
                        onClick={() =>
                          navigate({
                            name: "chat",
                            spaceId,
                            conversationId: conv.id,
                          })
                        }
                      >
                        <span className="page-activity__content">
                          <span className="page-activity__conversation-title">
                            {conv.title?.trim() || "Conversation"}
                          </span>
                          <span className="page-activity__meta">{conv.timeLabel}</span>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>

        <div
          id="new-chat-tabpanel-files"
          role="tabpanel"
          aria-labelledby="new-chat-tab-files"
          hidden={activeTab !== "files"}
          className="page-new-chat__tabpanel"
        >
          {activeTab === "files" && (
            <div className="page-new-chat__files-link">
              <p className="page-new-chat__files-copy">
                This space&apos;s working files live in Files, so an agent
                started here can already read anything uploaded there.
              </p>
              <button
                type="button"
                className="page-activity__action-btn"
                onClick={() => navigate({ name: "explore", spaceId })}
              >
                Open Files
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
