import type { Artifact, Conversation, ViewArtifactParams } from "../lib/types"
import { navigate } from "../router"

interface HomeProps {
  profileId: string
  conversations: Conversation[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

const RECENT_CONVERSATIONS = 5

export function Home({
  profileId,
  conversations,
  artifacts,
  onViewArtifact,
}: HomeProps) {
  const recentConversations = conversations.slice(0, RECENT_CONVERSATIONS)

  return (
    <div className="page-home">
      <section className="page-home__chats">
        <h2 className="page-home__heading">Recent conversations</h2>
        {recentConversations.length === 0 ? (
          <p className="page-home__empty">No conversations yet. Use New Conversation in the sidebar to start one.</p>
        ) : (
          <ul className="page-home__list">
            {recentConversations.map((conv) => (
              <li key={conv.id} className="page-home__chat-card">
                <button
                  type="button"
                  className="page-home__chat-link"
                  onClick={() =>
                    navigate({ name: "conversation", profileId, conversationId: conv.id })
                  }
                >
                  <span className="page-home__chat-name">
                    {conv.title?.trim() || "Conversation"}
                  </span>
                  <span className="page-home__chat-time">{conv.timeLabel}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="page-home__artifacts">
        <h2 className="page-home__heading">Recent artifacts</h2>
        {artifacts.length === 0 ? (
          <p className="page-home__empty">No artifacts yet.</p>
        ) : (
          <ul className="page-home__artifact-list">
            {artifacts.map((a) => (
              <li key={a.id} className="page-home__artifact-card">
                <div className="page-home__artifact-main">
                  <span className="page-home__artifact-title">{a.title}</span>
                  <span className="page-home__artifact-time">{a.timeLabel}</span>
                </div>
                {onViewArtifact && (
                  <button
                    type="button"
                    className="page-home__artifact-view"
                    onClick={() => onViewArtifact({ taskRunId: a.id })}
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
