import { useState } from "react"
import { navigate } from "../router"
import { getErrorMessage } from "../lib/errorMessage"
import { cn } from "../lib/cn"
import { createChat } from "../lib/api"
import { chatStatusIcon } from "../lib/taskStatus"
import { PromptArea } from "../components/PromptArea"
import { FilesPanel } from "../components/FilesPanel"
import type { Artifact, Chat, ViewArtifactParams } from "../lib/types"

type NewChatTab = "chats" | "artifacts" | "files"

interface NewChatProps {
  workspaceId: string
  token?: string
  onRefetchWorkspaceChats?: () => void
  workspaceChats: Chat[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function NewChat({
  workspaceId,
  token,
  onRefetchWorkspaceChats,
  workspaceChats,
  artifacts,
  onViewArtifact,
}: NewChatProps) {
  const [prompt, setPrompt] = useState("")
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<NewChatTab>("chats")

  async function handleRun() {
    const input = prompt.trim()
    if (!input || !token || running) return
    setRunning(true)
    setRunError(null)
    try {
      const chat = await createChat(workspaceId, { input }, token)
      setPrompt("")
      onRefetchWorkspaceChats?.()
      navigate({ name: "chat", workspaceId, chatId: chat.id })
    } catch (err) {
      setRunError(getErrorMessage(err, "Failed to start chat"))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="page-new-chat">
      <h1 className="page-new-chat__title">New Chat</h1>
      <p className="page-new-chat__subtitle">
        Start a new conversation. Describe what you want to accomplish and the agent will work on it.
      </p>
      <PromptArea
        value={prompt}
        onChange={(v) => { setPrompt(v); setRunError(null) }}
        onRun={handleRun}
        heading="What would you like to do?"
        placeholder="e.g. Help me analyze last month's sales data"
      />
      {runError && (
        <p className="page-new-chat__error" role="alert">
          {runError}
        </p>
      )}

      <div className="page-new-chat__tabs">
        <div className="page-new-chat__tab-list" role="tablist" aria-label="Chats, artifacts, and files">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "chats"}
            aria-controls="new-chat-tabpanel-chats"
            id="new-chat-tab-chats"
            className={cn("page-new-chat__tab", activeTab === "chats" && "page-new-chat__tab--active")}
            onClick={() => setActiveTab("chats")}
          >
            Chats
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
          id="new-chat-tabpanel-chats"
          role="tabpanel"
          aria-labelledby="new-chat-tab-chats"
          hidden={activeTab !== "chats"}
          className="page-new-chat__tabpanel"
        >
          {activeTab === "chats" && (
            <div className="page-new-chat__chats">
              {workspaceChats.length === 0 ? (
                <p className="page-activity__empty">No chats yet.</p>
              ) : (
                <ul className="page-activity__list">
                  {workspaceChats.map((chat) => (
                    <li key={chat.id} className="page-activity__item">
                      <button
                        type="button"
                        className="page-activity__link"
                        onClick={() =>
                          navigate({
                            name: "chat",
                            workspaceId,
                            chatId: chat.id,
                          })
                        }
                      >
                        <span className="page-activity__icon">
                          {chatStatusIcon(chat.status)}
                        </span>
                        <span className="page-activity__content">
                          <span className="page-activity__task-title">{chat.title}</span>
                          <span className="page-activity__meta">{chat.timeLabel}</span>
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
                          {a.timeLabel} · chat: {a.chatId} · artifact: {a.id}
                        </span>
                      </span>
                      {onViewArtifact && (
                        <button
                          type="button"
                          className="page-activity__artifact-view"
                          onClick={() => onViewArtifact({ workspaceId, artifactId: a.id })}
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
            <FilesPanel workspaceId={workspaceId} className="page-new-chat__files-panel" />
          )}
        </div>
      </div>
    </div>
  )
}
