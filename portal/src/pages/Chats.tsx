import { useCallback, useEffect, useState } from "react"
import type { Chat } from "../lib/types"
import { chatStatusIcon } from "../lib/chatStatus"
import { navigate } from "../router"
import { getAgents, apiAgentToAgent } from "../lib/api"
import type { Agent } from "../lib/types"

interface ChatsProps {
  workspaceId: string
  chats: Chat[]
  token: string | null
}

export function Chats({
  workspaceId,
  chats,
  token,
}: ChatsProps) {
  const [agents, setAgents] = useState<Agent[]>([])

  const fetchAgents = useCallback(() => {
    if (!token) return
    getAgents(workspaceId, token)
      .then((list) => setAgents(list.map(apiAgentToAgent)))
      .catch(() => setAgents([]))
  }, [workspaceId, token])

  useEffect(() => {
    fetchAgents()
  }, [fetchAgents])

  const agentNameById = new Map(agents.map((a) => [a.id, a.name]))

  return (
    <div className="page-activity">
      <h1 className="page-activity__title">Chats</h1>
      <p className="page-activity__subtitle">
        All chats in this workspace.
      </p>
      {chats.length === 0 ? (
        <p className="page-activity__empty">No chats yet.</p>
      ) : (
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
                  <span className="page-activity__meta">
                    {chat.timeLabel}
                    {chat.agentId && agentNameById.has(chat.agentId)
                      ? ` · Agent: ${agentNameById.get(chat.agentId)}`
                      : ""}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
