import {
  CreateEntityModal,
  type CreateEntityFieldConfig,
} from "./CreateEntityModal"

export const AGENT_FIELDS: CreateEntityFieldConfig[] = [
  {
    key: "name",
    label: "Name",
    type: "text",
    placeholder: "e.g. Code reviewer",
    maxLength: 200,
  },
  {
    key: "description",
    label: "Description",
    type: "text",
    placeholder: "Short description",
    optional: true,
    maxLength: 500,
  },
  {
    key: "instructions",
    label: "Instructions",
    type: "textarea",
    placeholder: "System instructions for this agent",
    optional: true,
    rows: 4,
  },
]

interface CreateAgentModalProps {
  open: boolean
  loading: boolean
  error: string | null
  onClose: () => void
  onCreate: (values: {
    name: string
    description?: string
    instructions?: string
  }) => void
}

export function CreateAgentModal({
  open,
  loading,
  error,
  onClose,
  onCreate,
}: CreateAgentModalProps) {
  return (
    <CreateEntityModal
      open={open}
      title="New Agent"
      titleId="create-agent-title"
      fields={AGENT_FIELDS}
      hint="Agents are personas or task templates you can use in this workspace."
      loading={loading}
      error={error}
      submitLabel="Create agent"
      onClose={onClose}
      onSubmit={(values) => {
        const name = values.name?.trim()
        if (!name) return
        onCreate({
          name,
          description: values.description?.trim() || undefined,
          instructions: values.instructions?.trim() || undefined,
        })
      }}
    />
  )
}
