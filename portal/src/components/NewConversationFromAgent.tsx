import { useState, useEffect } from "react"
import type { Agent } from "../lib/types"
import { BaseModal } from "@buildmax/gui"

/** Builds the agent preview including ID so Tier 1 can pass it to StartTask. */
export function buildAgentPreview(agent: Agent): string {
  return `Agent: ${agent.name} (id: ${agent.id})\nDescription: ${agent.description ?? ""}\nInstructions:\n${agent.instructions ?? ""}\n\nPlease start a background task with this agent.`
}

interface NewConversationFromAgentProps {
  open: boolean
  agent: Agent | null
  loading: boolean
  error: string | null
  onClose: () => void
  onStart: (input: string) => void
}

export function NewConversationFromAgent({
  open,
  agent,
  loading,
  error,
  onClose,
  onStart,
}: NewConversationFromAgentProps) {
  const [input, setInput] = useState("")

  useEffect(() => {
    if (open && agent) {
      setInput(buildAgentPreview(agent))
    }
  }, [open, agent])

  function handleSubmit() {
    onStart(input.trim())
  }

  if (!agent) return null

  return (
    <BaseModal
      open={open}
      title={`New conversation with ${agent.name}`}
      titleId="new-conversation-from-agent-title"
      onClose={onClose}
      className="modal--large"
    >
      <div className="modal__body">
        <p className="modal__hint" id="new-conversation-from-agent-hint">
          Review and edit the instructions below. You can add more context before starting.
        </p>
        <label className="modal__label" htmlFor="new-conversation-from-agent-input">
          Instructions
        </label>
        <textarea
          id="new-conversation-from-agent-input"
          className="modal__textarea new-conversation-from-agent-modal__textarea"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          rows={10}
          disabled={loading}
          placeholder="Agent: ..."
          aria-describedby="new-conversation-from-agent-hint"
        />
        {error ? (
          <p className="modal__error" id="new-conversation-from-agent-error" role="alert">
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
          {loading ? "Starting…" : "Start"}
        </button>
      </div>
    </BaseModal>
  )
}
