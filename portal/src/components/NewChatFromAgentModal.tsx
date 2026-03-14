import { useState, useEffect } from "react"
import type { Agent } from "../lib/types"
import { BaseModal } from "@buildmax/gui"

/** Builds the same preview format the backend uses for agent-based chat input. */
export function buildAgentPreview(agent: Agent): string {
  return `Agent: ${agent.name}\nDescription: ${agent.description ?? ""}\nInstructions:\n${agent.instructions ?? ""}`
}

interface NewChatFromAgentModalProps {
  open: boolean
  agent: Agent | null
  loading: boolean
  error: string | null
  onClose: () => void
  onStartChat: (input: string) => void
}

export function NewChatFromAgentModal({
  open,
  agent,
  loading,
  error,
  onClose,
  onStartChat,
}: NewChatFromAgentModalProps) {
  const [input, setInput] = useState("")

  useEffect(() => {
    if (open && agent) {
      setInput(buildAgentPreview(agent))
    }
  }, [open, agent])

  function handleSubmit() {
    onStartChat(input.trim())
  }

  if (!agent) return null

  return (
    <BaseModal
      open={open}
      title={`New chat with ${agent.name}`}
      titleId="new-chat-from-agent-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <p className="modal__hint" id="new-chat-from-agent-hint">
          Review and edit the instructions below. You can add more context before starting the chat.
        </p>
        <label className="modal__label" htmlFor="new-chat-from-agent-input">
          Instructions
        </label>
        <textarea
          id="new-chat-from-agent-input"
          className="modal__textarea new-chat-from-agent-modal__textarea"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          rows={10}
          disabled={loading}
          placeholder="Agent: ..."
          aria-describedby="new-chat-from-agent-hint"
        />
        {error ? (
          <p className="modal__error" id="new-chat-from-agent-error" role="alert">
            {error}
          </p>
        ) : null}
      </div>
      <div className="modal__actions">
        <button
          type="button"
          className="modal__btn modal__btn--secondary"
          onClick={onClose}
          disabled={loading}
        >
          Cancel
        </button>
        <button
          type="button"
          className="modal__btn modal__btn--secondary"
          onClick={handleSubmit}
          disabled={loading}
        >
          {loading ? "Starting…" : "Start chat"}
        </button>
      </div>
    </BaseModal>
  )
}
