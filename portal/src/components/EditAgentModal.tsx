import type { ReactNode } from "react"
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
    sandbox_network_tier?: string
    sandbox_filesystem_tier?: string
  }) => void
  onDelete: () => void
  history?: ReactNode
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
  history,
}: EditAgentModalProps) {
  const initialValues =
    agent != null
      ? {
          name: agent.name,
          description: agent.description ?? "",
          instructions: agent.instructions ?? "",
          sandbox_network_tier: agent.sandboxNetworkTier ?? "",
          sandbox_filesystem_tier: agent.sandboxFilesystemTier ?? "",
        }
      : undefined

  function handleDelete() {
    if (
      window.confirm(
        "Delete this agent? It leaves the team and cannot be restored. Runs, tasks, and history that already reference it stay readable.",
      )
    ) {
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
      hint="Agents are personas or task templates you can use across your account."
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
          sandbox_network_tier: values.sandbox_network_tier || undefined,
          sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
        })
      }}
    >
      {history}
    </FormModal>
  )
}
