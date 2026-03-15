import { useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { ChatComposer } from "@buildmax/gui"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import { cn } from "../lib/cn"
import { createConversationStream } from "../features/conversations"
import { FilesPanel } from "../components/FilesPanel"
import type { Artifact, Conversation, ViewArtifactParams } from "../lib/types"

type NewConversationTab = "conversations" | "artifacts" | "files"

interface NewConversationProps {
  profileId: string
  token?: string
  onRefetchConversations?: () => void
  conversations: Conversation[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function NewConversation({
  profileId,
  token,
  onRefetchConversations,
  conversations,
  artifacts,
  onViewArtifact,
}: NewConversationProps) {
  const [prompt, setPrompt] = useState("")
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<NewConversationTab>("conversations")
  const [streamingConversationId, setStreamingConversationId] = useState<string | null>(null)
  const [streamingContent, setStreamingContent] = useState("")
  const conversationIdRef = useRef<string | null>(null)
  const streamHadErrorRef = useRef(false)

  async function handleSend() {
    const input = prompt.trim()
    if (!input || !token || running) return
    setRunning(true)
    setRunError(null)
    setStreamingConversationId(null)
    setStreamingContent("")
    conversationIdRef.current = null
    streamHadErrorRef.current = false
    try {
      await createConversationStream(
        profileId,
        { channel: "portal", message: input },
        token,
        {
          onConversationId: (id) => {
            conversationIdRef.current = id
            setStreamingConversationId(id)
          },
          onDelta: (delta) => setStreamingContent((prev) => prev + delta),
          onDone: () => {
            setPrompt("")
            setRunning(false)
            setStreamingContent("")
            setStreamingConversationId(null)
            if (!streamHadErrorRef.current) {
              const id = conversationIdRef.current
              if (id) {
                navigate({ name: "conversation", profileId, conversationId: id })
                onRefetchConversations?.()
              }
            }
          },
          onError: (err) => {
            streamHadErrorRef.current = true
            setRunError(getErrorMessage(err, "Failed to start conversation"))
            setRunning(false)
            setStreamingContent("")
            setStreamingConversationId(null)
          },
        }
      )
    } catch (err) {
      setRunError(getErrorMessage(err, "Failed to start conversation"))
      setRunning(false)
      setStreamingContent("")
      setStreamingConversationId(null)
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
        {(streamingContent.length > 0 || (running && streamingConversationId)) && (
          <div className="page-chat__message page-chat__message--assistant" aria-live="polite">
            <span className="page-chat__message-role">assistant</span>
            <div className="page-chat__message-content">
              {streamingContent ? (
                <Markdown remarkPlugins={[remarkGfm]}>{streamingContent}</Markdown>
              ) : (
                <p className="page-chat__text page-chat__muted">Thinking…</p>
              )}
            </div>
          </div>
        )}
      </section>

      <div className="page-new-chat__tabs">
        <div className="page-new-chat__tab-list" role="tablist" aria-label="Conversations, artifacts, and files">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "conversations"}
            aria-controls="new-chat-tabpanel-conversations"
            id="new-chat-tab-conversations"
            className={cn("page-new-chat__tab", activeTab === "conversations" && "page-new-chat__tab--active")}
            onClick={() => setActiveTab("conversations")}
          >
            Conversations
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "artifacts"}
            aria-controls="new-chat-tabpanel-artifacts"
            id="new-chat-tab-artifacts"
            className={cn("page-new-chat__tab", activeTab === "artifacts" && "page-new-chat__tab--active")}
            onClick={() => setActiveTab("artifacts")}
          >
            Artifacts
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
              {conversations.length === 0 ? (
                <p className="page-activity__empty">No conversations yet.</p>
              ) : (
                <ul className="page-activity__list">
                  {conversations.map((conv) => (
                    <li key={conv.id} className="page-activity__item">
                      <button
                        type="button"
                        className="page-activity__link"
                        onClick={() =>
                          navigate({
                            name: "conversation",
                            profileId,
                            conversationId: conv.id,
                          })
                        }
                      >
                        <span className="page-activity__content">
                          <span className="page-activity__task-title">
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
          id="new-chat-tabpanel-artifacts"
          role="tabpanel"
          aria-labelledby="new-chat-tab-artifacts"
          hidden={activeTab !== "artifacts"}
          className="page-new-chat__tabpanel"
        >
          {activeTab === "artifacts" && (
            <div className="page-new-chat__chats">
              {artifacts.length === 0 ? (
                <p className="page-activity__empty">No artifacts yet.</p>
              ) : (
                <ul className="page-activity__artifact-list">
                  {artifacts.map((a) => (
                    <li key={`artifact-${a.id}`} className="page-activity__artifact-item">
                      <span className="page-activity__content">
                        <span className="page-activity__task-title">{a.title}</span>
                        <span className="page-activity__meta">
                          {a.timeLabel} · task: {a.taskId} · artifact: {a.id}
                        </span>
                      </span>
                      {onViewArtifact && (
                        <button
                          type="button"
                          className="page-activity__artifact-view"
                          onClick={() => onViewArtifact({ taskRunId: a.id })}
                        >
                          View
                        </button>
                      )}
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
          className="page-new-chat__tabpanel page-new-chat__tabpanel--files"
        >
          {activeTab === "files" && (
            <FilesPanel profileId={profileId} className="page-new-chat__files-panel" />
          )}
        </div>
      </div>
    </div>
  )
}
