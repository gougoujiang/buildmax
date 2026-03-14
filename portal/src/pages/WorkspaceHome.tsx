import type { Artifact, Conversation, ViewArtifactParams } from "../lib/types"
import { navigate } from "../router"

interface WorkspaceHomeProps {
  workspaceId: string
  workspaceConversations: Conversation[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

const RECENT_CONVERSATIONS = 5

export function WorkspaceHome({
  workspaceId,
  workspaceConversations,
  artifacts,
  onViewArtifact,
}: WorkspaceHomeProps) {
  const recentConversations = workspaceConversations.slice(0, RECENT_CONVERSATIONS)

  return (
    <div className="page-workspace">
      <section className="page-workspace__chats">
        <h2 className="page-workspace__heading">Recent conversations</h2>
        {recentConversations.length === 0 ? (
          <p className="page-workspace__empty">No conversations yet. Use New Chat in the sidebar to start one.</p>
        ) : (
          <ul className="page-workspace__list">
            {recentConversations.map((conv) => (
              <li key={conv.id} className="page-workspace__chat-card">
                <button
                  type="button"
                  className="page-workspace__chat-link"
                  onClick={() =>
                    navigate({ name: "conversation", workspaceId, conversationId: conv.id })
                  }
                >
                  <span className="page-workspace__chat-name">
                    {conv.title?.trim() || "Conversation"}
                  </span>
                  <span className="page-workspace__chat-time">{conv.timeLabel}</span>
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
                    onClick={() => onViewArtifact({ workspaceId, chatRunId: a.id })}
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
