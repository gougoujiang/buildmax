import type { Conversation } from "../lib/types"
import { navigate } from "../router"

interface TasksProps {
  conversations: Conversation[]
}

export function Tasks({
  conversations,
}: TasksProps) {
  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Conversations</h1>
      <p className="page-activity__subtitle">
        All conversations for this view.
      </p>
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
  )
}
