import { FormModal, type FormModalFieldConfig } from "@buildmax/gui"

const WORKSPACE_FIELDS: FormModalFieldConfig[] = [
  {
    key: "name",
    label: "Workspace name",
    type: "text",
    placeholder: "e.g. Marketing, Engineering, Personal",
    maxLength: 100,
  },
]

interface CreateWorkspaceModalProps {
  open: boolean
  loading: boolean
  error: string | null
  onClose: () => void
  onCreate: (name: string) => void
}

export function CreateWorkspaceModal({
  open,
  loading,
  error,
  onClose,
  onCreate,
}: CreateWorkspaceModalProps) {
  return (
    <FormModal
      open={open}
      title="New Workspace"
      titleId="create-ws-title"
      fields={WORKSPACE_FIELDS}
      hint="Give your workspace a name that describes its purpose. You can always create more later."
      loading={loading}
      error={error}
      submitLabel="Create workspace"
      onClose={onClose}
      onSubmit={(values) => values.name?.trim() && onCreate(values.name.trim())}
    />
  )
}
