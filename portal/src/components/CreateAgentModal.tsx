import { FormModal, type FormModalFieldConfig } from "@buildmax/gui"

export const AGENT_FIELDS: FormModalFieldConfig[] = [
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
    <FormModal
      open={open}
      title="New Agent"
      titleId="create-agent-title"
      fields={AGENT_FIELDS}
      hint="Agents are personas or task templates you can use across your account."
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
