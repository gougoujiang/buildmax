import { useState } from "react"
import type { Artifact, Chat, ViewArtifactParams } from "../lib/types"
import { navigate } from "../router"
import { createChat } from "../lib/api"
import { chatStatusIcon } from "../lib/taskStatus"
import { PromptArea } from "../components/PromptArea"

interface WorkspaceHomeProps {
  workspaceId: string
  workspaceChats: Chat[]
  artifacts: Artifact[]
  token?: string
  onRefetchWorkspaceChats?: () => void
  onViewArtifact?: (params: ViewArtifactParams) => void
}

const RECENT_CHATS = 5

export function WorkspaceHome({
  workspaceId,
  workspaceChats,
  artifacts,
  token,
  onRefetchWorkspaceChats,
  onViewArtifact,
}: WorkspaceHomeProps) {
  const [prompt, setPrompt] = useState("")
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)

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
      setRunError(err instanceof Error ? err.message : "Failed to start chat")
    } finally {
      setRunning(false)
    }
  }

  const recentChats = workspaceChats.slice(0, RECENT_CHATS)

  return (
    <div className="page-workspace">
      <PromptArea
        value={prompt}
        onChange={(v) => { setPrompt(v); setRunError(null) }}
        onRun={handleRun}
        heading="What would you like to accomplish?"
      />
      {runError && (
        <p className="page-workspace__error" role="alert">
          {runError}
        </p>
      )}

      <section className="page-workspace__chats">
        <h2 className="page-workspace__heading">Recent chats</h2>
        {recentChats.length === 0 ? (
          <p className="page-workspace__empty">No chats yet. Start one above or use New Chat in the sidebar.</p>
        ) : (
          <ul className="page-workspace__list">
            {recentChats.map((chat) => (
              <li key={chat.id} className="page-workspace__chat-card">
                <button
                  type="button"
                  className="page-workspace__chat-link"
                  onClick={() =>
                    navigate({ name: "chat", workspaceId, chatId: chat.id })
                  }
                >
                  <span className="page-workspace__chat-name">
                    {chat.title?.trim() || "New chat"}
                  </span>
                  <span className="page-workspace__chat-status">
                    {chatStatusIcon(chat.status)}
                  </span>
                  <span className="page-workspace__chat-time">
                    {chat.timeLabel}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="page-workspace__artifacts">
        <h2 className="page-workspace__heading">Recent artifacts</h2>
        {artifacts.length === 0 ? (
          <p className="page-workspace__empty">No artifacts yet.</p>
        ) : (
          <ul className="page-workspace__artifact-list">
            {artifacts.map((a) => (
              <li key={a.id} className="page-workspace__artifact-card">
                <div className="page-workspace__artifact-main">
                  <span className="page-workspace__artifact-title">{a.title}</span>
                  <span className="page-workspace__artifact-time">{a.timeLabel}</span>
                </div>
                {onViewArtifact && (
                  <button
                    type="button"
                    className="page-workspace__artifact-view"
                    onClick={() => onViewArtifact({ workspaceId, artifactId: a.id })}
                  >
                    View
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
