import type { Agent } from "../lib/types"
import { FormModal } from "@buildmax/gui"
import { AGENT_FIELDS } from "./CreateAgentModal"

interface EditAgentModalProps {
  open: boolean
  agent: Agent | null
  loading: boolean
  error: string | null
  deleting?: boolean
  onClose: () => void
  onSave: (values: {
    name: string
    description?: string
    instructions?: string
  }) => void
  onDelete: () => void
}

export function EditAgentModal({
  open,
  agent,
  loading,
  error,
  deleting = false,
  onClose,
  onSave,
  onDelete,
}: EditAgentModalProps) {
  const initialValues =
    agent != null
      ? {
          name: agent.name,
          description: agent.description ?? "",
          instructions: agent.instructions ?? "",
        }
      : undefined

  function handleDelete() {
    if (window.confirm("Delete this agent? This cannot be undone.")) {
      onDelete()
    }
  }

  if (agent == null) return null

  return (
    <FormModal
      open={open}
      title="Edit agent"
      titleId="edit-agent-title"
      fields={AGENT_FIELDS}
      hint="Agents are personas or task templates you can use in this workspace."
      initialValues={initialValues}
      className="modal--large"
      dangerAction={{
        label: "Delete",
        onClick: handleDelete,
        disabled: deleting,
      }}
      loading={loading}
      error={error}
      submitLabel="Save"
      onClose={onClose}
      onSubmit={(values) => {
        const name = values.name?.trim()
        if (!name) return
        onSave({
          name,
          description: values.description?.trim() || undefined,
          instructions: values.instructions?.trim() || undefined,
        })
      }}
    />
  )
}
