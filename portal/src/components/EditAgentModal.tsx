import { useEffect, useState, type ReactNode } from "react"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import type { Agent } from "../lib/types"
import { FormModal } from "@buildmax/gui"
import { AGENT_FIELDS, buildAgentGroups } from "./CreateAgentModal"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"
import { PluginSelectionEditor } from "./PluginSelectionEditor"

interface EditAgentModalProps {
  open: boolean
  agent: Agent | null
  secrets: ApiSecret[]
  availablePlugins: string[]
  loading: boolean
  error: string | null
  deleting?: boolean
  onClose: () => void
  onSave: (values: {
    name: string
    description?: string
    instructions?: string
    plugins?: string[]
    sandbox_network_tier?: string
    sandbox_filesystem_tier?: string
    secret_consumption?: ApiSecretConsumption
  }) => void
  onDelete: () => void
  history?: ReactNode
}

export function EditAgentModal({
  open,
  agent,
  secrets,
  availablePlugins,
  loading,
  error,
  deleting = false,
  onClose,
  onSave,
  onDelete,
  history,
}: EditAgentModalProps) {
  const [consumption, setConsumption] = useState<ApiSecretConsumption>({})
  const [plugins, setPlugins] = useState<string[]>([])

  // Reset both editors whenever a different agent is opened.
  useEffect(() => {
    setConsumption(agent?.secretConsumption ?? {})
    setPlugins(agent?.plugins ?? [])
  }, [agent])

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

  const groups = buildAgentGroups({
    pluginEditor: (
      <PluginSelectionEditor value={plugins} onChange={setPlugins} available={availablePlugins} />
    ),
    secretEditor: (
      <SecretConsumptionEditor value={consumption} onChange={setConsumption} secrets={secrets} />
    ),
    open: {
      sandbox: Boolean(agent.sandboxNetworkTier || agent.sandboxFilesystemTier),
      plugins: (agent.plugins?.length ?? 0) > 0,
      secrets: (agent.secretConsumption?.env?.length ?? 0) > 0,
    },
  })

  return (
    <FormModal
      open={open}
      title="Edit agent"
      titleId="edit-agent-title"
      fields={AGENT_FIELDS}
      groups={groups}
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
          plugins,
          sandbox_network_tier: values.sandbox_network_tier || undefined,
          sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
          secret_consumption: normalizeConsumption(consumption),
        })
      }}
    >
      {history}
    </FormModal>
  )
}

/**
 * normalizeConsumption drops incomplete grants -- a row with no secret chosen,
 * or a selected item with no variable name -- so a half-filled row does not
 * reach the API. An empty result is sent as an empty object, which clears the
 * agent's consumption (the PATCH replaces the whole definition).
 */
export function normalizeConsumption(c: ApiSecretConsumption): ApiSecretConsumption {
  const env = (c.env ?? []).filter((g) => {
    if (!g.secret) return false
    if (g.item) return Boolean(g.env_name)
    return true
  })
  return { env }
}
