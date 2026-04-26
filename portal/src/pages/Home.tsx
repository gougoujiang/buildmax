import type { Conversation } from "../lib/types"
import { navigate } from "../router"

interface HomeProps {
  conversations: Conversation[]
}

const RECENT_CONVERSATIONS = 5

export function Home({
  conversations,
}: HomeProps) {
  const recentConversations = conversations.slice(0, RECENT_CONVERSATIONS)

  return (
    <div className="page-home">
      <section className="page-home__chats">
        <h2 className="page-home__heading">Recent conversations</h2>
        {recentConversations.length === 0 ? (
          <p className="page-home__empty">No conversations yet. Use Conversations in the sidebar to start one.</p>
        ) : (
          <ul className="page-home__list">
            {recentConversations.map((conv) => (
              <li key={conv.id} className="page-home__chat-card">
                <button
                  type="button"
                  className="page-home__chat-link"
                  onClick={() => navigate({ name: "conversation", conversationId: conv.id })}
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
    </div>
  )
}
