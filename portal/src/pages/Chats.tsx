import type { Artifact, Chat, ViewArtifactParams } from "../lib/types"
import { chatStatusIcon } from "../lib/chatStatus"
import { navigate } from "../router"

interface ChatsProps {
  workspaceId: string
  chats: Chat[]
  artifacts: Artifact[]
  onViewArtifact?: (params: ViewArtifactParams) => void
}

export function Chats({
  workspaceId,
  chats,
  artifacts,
  onViewArtifact,
}: ChatsProps) {
  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Chats</h1>
      <p className="page-activity__subtitle">
        All chats and artifacts in this workspace.
      </p>
      {chats.length === 0 && artifacts.length === 0 ? (
        <p className="page-activity__empty">No chats yet.</p>
      ) : (
        <>
          {chats.length > 0 && (
            <ul className="page-activity__list">
              {chats.map((chat) => (
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
          {artifacts.length > 0 && (
            <section className="page-activity__artifacts">
              <h2 className="page-activity__artifacts-heading">Artifacts</h2>
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
                        onClick={() => onViewArtifact({ workspaceId, chatRunId: a.id })}
                      >
                        View
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </div>
  )
}
