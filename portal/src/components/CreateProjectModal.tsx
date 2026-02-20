import {
  CreateEntityModal,
  type CreateEntityFieldConfig,
} from "./CreateEntityModal"

const PROJECT_FIELDS: CreateEntityFieldConfig[] = [
  {
    key: "name",
    label: "Project name",
    type: "text",
    placeholder: "e.g. Q1 Report, Website Redesign, Data Analysis",
    maxLength: 100,
  },
  {
    key: "description",
    label: "Description",
    type: "textarea",
    placeholder: "What is this project about?",
    optional: true,
    maxLength: 500,
    rows: 3,
  },
]

interface CreateProjectModalProps {
  open: boolean
  loading: boolean
  error: string | null
  onClose: () => void
  onCreate: (name: string, description: string) => void
}

export function CreateProjectModal({
  open,
  loading,
  error,
  onClose,
  onCreate,
}: CreateProjectModalProps) {
  return (
    <CreateEntityModal
      open={open}
      title="New Project"
      titleId="create-proj-title"
      fields={PROJECT_FIELDS}
      loading={loading}
      error={error}
      submitLabel="Create project"
      onClose={onClose}
      onSubmit={(values) =>
        values.name?.trim() && onCreate(values.name.trim(), values.description?.trim() ?? "")
      }
    />
  )
}
