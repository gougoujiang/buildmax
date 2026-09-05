import { useState } from "react"
import { FormModal } from "@buildmax/gui"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import { agentFields, buildAgentDefinition, buildAgentGroups } from "../features/agents"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"
import { PluginSelectionEditor } from "./PluginSelectionEditor"

interface CreateAgentModalProps {
  open: boolean
  loading: boolean
  error: string | null
  secrets: ApiSecret[]
  availablePlugins: string[]
  availableModels: string[]
  onClose: () => void
  onCreate: (values: {
    name: string
    description?: string
    instructions?: string
    model?: string
    plugins?: string[]
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
  availablePlugins,
  availableModels,
  onClose,
  onCreate,
}: CreateAgentModalProps) {
  const [consumption, setConsumption] = useState<ApiSecretConsumption>({})
  const [plugins, setPlugins] = useState<string[]>([])
  const groups = buildAgentGroups({
    pluginEditor: (
      <PluginSelectionEditor value={plugins} onChange={setPlugins} available={availablePlugins} />
    ),
    secretEditor: (
      <SecretConsumptionEditor value={consumption} onChange={setConsumption} secrets={secrets} />
    ),
  })
  return (
    <FormModal
      open={open}
      title="New Agent"
      titleId="create-agent-title"
      fields={agentFields(availableModels)}
      groups={groups}
      layout="tabs"
      hint="Agents are personas or task templates you can use across your account."
      loading={loading}
      error={error}
      submitLabel="Create agent"
      onClose={onClose}
      onSubmit={(values) => {
        const definition = buildAgentDefinition(values, plugins, consumption)
        if (definition == null) return
        onCreate(definition)
      }}
    />
  )
}
