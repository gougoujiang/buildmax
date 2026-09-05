import { useState, type ReactNode } from "react"
import { FormModal, type FormModalGroup } from "@buildmax/gui"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import { agentFields, buildAgentDefinition } from "../features/agents"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"
import { PluginSelectionEditor } from "./PluginSelectionEditor"

// Section metadata shared by the create dialog. The dialog lays these out as
// tabs (a left sidebar), so every section is one click away and the dialog's
// height never runs away with a long Instructions field or the history. The
// plugin and secret groups' content (their editors) is injected per dialog via
// buildAgentGroups, because it needs the dialog's live state.
const AGENT_GROUP_META: FormModalGroup[] = [
  { id: "basics", title: "Basics" },
  {
    id: "sandbox",
    title: "Sandbox access",
    description:
      "Restrict what this agent's runs can reach. Leave on the team default unless this agent needs something different.",
  },
  {
    id: "plugins",
    title: "Plugins",
    description:
      "Catalog plugins this agent loads for background runs. Nothing is inherited — an agent that names none loads none.",
  },
  {
    id: "secrets",
    title: "Secrets",
    description: "Grant Team Secrets to this agent's runs as environment variables.",
  },
]

/**
 * buildAgentGroups injects the live plugin and secret editors into their groups.
 * Kept exported because the dialog's tab layout is built from it.
 */
export function buildAgentGroups(opts: {
  pluginEditor: ReactNode
  secretEditor: ReactNode
}): FormModalGroup[] {
  return AGENT_GROUP_META.map((group) => {
    if (group.id === "plugins") return { ...group, content: opts.pluginEditor }
    if (group.id === "secrets") return { ...group, content: opts.secretEditor }
    return group
  })
}

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
