import { useState } from "react"
import { FormModal, type FormModalFieldConfig } from "@buildmax/gui"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import {
  AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
} from "../lib/sandboxTiers"
import { normalizeConsumption } from "./EditAgentModal"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"

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
  {
    key: "sandbox_network_tier",
    label: "Network access",
    type: "select",
    optional: true,
    options: AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
  },
  {
    key: "sandbox_filesystem_tier",
    label: "Filesystem access",
    type: "select",
    optional: true,
    options: AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  },
]

interface CreateAgentModalProps {
  open: boolean
  loading: boolean
  error: string | null
  secrets: ApiSecret[]
  onClose: () => void
  onCreate: (values: {
    name: string
    description?: string
    instructions?: string
    sandbox_network_tier?: string
    sandbox_filesystem_tier?: string
    secret_consumption?: ApiSecretConsumption
  }) => void
}

export function CreateAgentModal({
  open,
  loading,
  error,
  secrets,
  onClose,
  onCreate,
}: CreateAgentModalProps) {
  const [consumption, setConsumption] = useState<ApiSecretConsumption>({})
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
          sandbox_network_tier: values.sandbox_network_tier || undefined,
          sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
          secret_consumption: normalizeConsumption(consumption),
        })
      }}
    >
      <SecretConsumptionEditor value={consumption} onChange={setConsumption} secrets={secrets} />
    </FormModal>
  )
}
