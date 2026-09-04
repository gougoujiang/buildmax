import { useState, type ReactNode } from "react"
import { FormModal, type FormModalFieldConfig, type FormModalGroup } from "@buildmax/gui"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import {
  AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
} from "../lib/sandboxTiers"
import { normalizeConsumption } from "./EditAgentModal"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"
import { PluginSelectionEditor } from "./PluginSelectionEditor"

export const AGENT_FIELDS: FormModalFieldConfig[] = [
  {
    key: "name",
    label: "Name",
    type: "text",
    placeholder: "e.g. Code reviewer",
    maxLength: 200,
    group: "basics",
  },
  {
    key: "description",
    label: "Description",
    type: "text",
    placeholder: "Short description",
    optional: true,
    maxLength: 500,
    group: "basics",
  },
  {
    key: "instructions",
    label: "Instructions",
    type: "textarea",
    placeholder: "System instructions for this agent",
    optional: true,
    rows: 4,
    group: "basics",
  },
  {
    key: "sandbox_network_tier",
    label: "Network access",
    type: "select",
    optional: true,
    options: AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
    group: "sandbox",
  },
  {
    key: "sandbox_filesystem_tier",
    label: "Filesystem access",
    type: "select",
    optional: true,
    options: AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
    group: "sandbox",
  },
]

// Section metadata shared by the create and edit dialogs. Basics stays open;
// the sandbox, plugin, and secret groups collapse so the dialog opens compact
// and only the essentials show until the user asks for the rest. The plugin and
// secret groups' content (their editors) is injected per dialog via
// buildAgentGroups, because it needs the dialog's live state.
const AGENT_GROUP_META: FormModalGroup[] = [
  { id: "basics", title: "Basics" },
  {
    id: "sandbox",
    title: "Sandbox access",
    description:
      "Restrict what this agent's runs can reach. Leave on the team default unless this agent needs something different.",
    collapsible: true,
  },
  {
    id: "plugins",
    title: "Plugins",
    description:
      "Catalog plugins this agent loads for background runs. Nothing is inherited — an agent that names none loads none.",
    collapsible: true,
  },
  {
    id: "secrets",
    title: "Secrets",
    description: "Grant Team Secrets to this agent's runs as environment variables.",
    collapsible: true,
  },
]

/**
 * buildAgentGroups injects the live plugin and secret editors into their groups
 * and lets a dialog open specific collapsible groups. The edit dialog opens the
 * sandbox, plugin, and secret groups when the agent already has non-default
 * values there, so editing never hides settings that are actually set.
 */
export function buildAgentGroups(opts: {
  pluginEditor: ReactNode
  secretEditor: ReactNode
  open?: { sandbox?: boolean; plugins?: boolean; secrets?: boolean }
}): FormModalGroup[] {
  const open = opts.open ?? {}
  return AGENT_GROUP_META.map((group) => {
    if (group.id === "sandbox") return { ...group, defaultOpen: open.sandbox ?? false }
    if (group.id === "plugins") {
      return { ...group, content: opts.pluginEditor, defaultOpen: open.plugins ?? false }
    }
    if (group.id === "secrets") {
      return { ...group, content: opts.secretEditor, defaultOpen: open.secrets ?? false }
    }
    return group
  })
}

interface CreateAgentModalProps {
  open: boolean
  loading: boolean
  error: string | null
  secrets: ApiSecret[]
  availablePlugins: string[]
  onClose: () => void
  onCreate: (values: {
    name: string
    description?: string
    instructions?: string
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
      fields={AGENT_FIELDS}
      groups={groups}
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
          plugins: plugins.length > 0 ? plugins : undefined,
          sandbox_network_tier: values.sandbox_network_tier || undefined,
          sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
          secret_consumption: normalizeConsumption(consumption),
        })
      }}
    />
  )
}
