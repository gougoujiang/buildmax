import { useEffect, useState } from "react"
import type { FormModalFieldConfig } from "@buildmax/gui"
import type { Agent } from "../lib/types"
import type { ApiSecret, ApiSecretConsumption } from "../lib/api/types"
import { AGENT_FIELDS, buildAgentDefinition, type AgentDefinitionInput } from "../features/agents"
import { SecretConsumptionEditor } from "./SecretConsumptionEditor"
import { PluginSelectionEditor } from "./PluginSelectionEditor"

interface AgentConfigFormProps {
  agent: Agent
  secrets: ApiSecret[]
  availablePlugins: string[]
  canManage: boolean
  saving: boolean
  deleting: boolean
  error: string | null
  onSave: (definition: AgentDefinitionInput) => void
  onDelete: () => void
}

function seedValues(agent: Agent): Record<string, string> {
  return {
    name: agent.name,
    description: agent.description ?? "",
    instructions: agent.instructions ?? "",
    sandbox_network_tier: agent.sandboxNetworkTier ?? "",
    sandbox_filesystem_tier: agent.sandboxFilesystemTier ?? "",
  }
}

/**
 * The agent's configuration as an inline page form — the same field set and
 * assembly rules the create dialog uses (via AGENT_FIELDS / buildAgentDefinition),
 * rendered for the detail page's Configuration tab instead of a modal. Seeding is
 * keyed on the agent's revision, so a save or restore re-seeds to the saved
 * values while an in-progress edit is never clobbered by an unrelated re-render.
 */
export function AgentConfigForm({
  agent,
  secrets,
  availablePlugins,
  canManage,
  saving,
  deleting,
  error,
  onSave,
  onDelete,
}: AgentConfigFormProps) {
  const [values, setValues] = useState<Record<string, string>>(() => seedValues(agent))
  const [plugins, setPlugins] = useState<string[]>(agent.plugins ?? [])
  const [consumption, setConsumption] = useState<ApiSecretConsumption>(agent.secretConsumption ?? {})

  useEffect(() => {
    setValues(seedValues(agent))
    setPlugins(agent.plugins ?? [])
    setConsumption(agent.secretConsumption ?? {})
    // Re-seed only when the authoritative version changes (save / restore),
    // never on every keystroke-driven re-render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent.id, agent.revision])

  function setField(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  function handleSubmit() {
    const definition = buildAgentDefinition(values, plugins, consumption)
    if (definition == null) return
    onSave(definition)
  }

  function handleDelete() {
    if (
      window.confirm(
        "Delete this agent? It leaves the team and cannot be restored. Runs, tasks, and history that already reference it stay readable.",
      )
    ) {
      onDelete()
    }
  }

  const disabled = !canManage || saving

  function renderField(field: FormModalFieldConfig) {
    const value = values[field.key] ?? ""
    return (
      <div key={field.key} className="agent-config__field">
        <label className="modal__label" htmlFor={`agent-cfg-${field.key}`}>
          {field.label}
          {field.optional ? <span className="modal__optional"> (optional)</span> : null}
        </label>
        {field.type === "textarea" ? (
          <textarea
            id={`agent-cfg-${field.key}`}
            className="modal__textarea"
            placeholder={field.placeholder}
            value={value}
            rows={field.rows ?? 4}
            maxLength={field.maxLength}
            disabled={disabled}
            onChange={(e) => setField(field.key, e.target.value)}
          />
        ) : field.type === "select" ? (
          <>
            <select
              id={`agent-cfg-${field.key}`}
              className="modal__input"
              value={value}
              disabled={disabled}
              onChange={(e) => setField(field.key, e.target.value)}
            >
              {(field.options ?? []).map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            {field.options?.find((o) => o.value === value)?.description ? (
              <p className="modal__hint">{field.options.find((o) => o.value === value)?.description}</p>
            ) : null}
          </>
        ) : (
          <input
            id={`agent-cfg-${field.key}`}
            type="text"
            className="modal__input"
            placeholder={field.placeholder}
            value={value}
            maxLength={field.maxLength}
            autoComplete="off"
            disabled={disabled}
            onChange={(e) => setField(field.key, e.target.value)}
          />
        )}
      </div>
    )
  }

  const basics = AGENT_FIELDS.filter((f) => f.group === "basics")
  const sandbox = AGENT_FIELDS.filter((f) => f.group === "sandbox")
  const nameEmpty = !values.name?.trim()

  return (
    <div className="agent-config">
      <section className="agent-config__card">
        <h3 className="agent-config__card-title">Basics</h3>
        {basics.map(renderField)}
      </section>

      <section className="agent-config__card">
        <h3 className="agent-config__card-title">Sandbox access</h3>
        <p className="modal__hint">
          Restrict what this agent's runs can reach. Leave on the team default unless this agent needs
          something different.
        </p>
        {sandbox.map(renderField)}
      </section>

      <section className="agent-config__card">
        <h3 className="agent-config__card-title">Plugins</h3>
        <p className="modal__hint">
          Catalog plugins this agent loads for background runs. Nothing is inherited — an agent that names
          none loads none.
        </p>
        <PluginSelectionEditor value={plugins} onChange={setPlugins} available={availablePlugins} />
      </section>

      <section className="agent-config__card">
        <h3 className="agent-config__card-title">Secrets</h3>
        <SecretConsumptionEditor value={consumption} onChange={setConsumption} secrets={secrets} />
      </section>

      {error ? (
        <p className="modal__error" role="alert">
          {error}
        </p>
      ) : null}

      {canManage ? (
        <div className="agent-config__actions">
          <button
            type="button"
            className="page-activity__action-btn"
            disabled={disabled || nameEmpty}
            onClick={handleSubmit}
          >
            {saving ? "Saving…" : "Save changes"}
          </button>
          <button
            type="button"
            className="agent-config__delete"
            disabled={deleting || saving}
            onClick={handleDelete}
          >
            {deleting ? "Deleting…" : "Delete agent"}
          </button>
        </div>
      ) : (
        <p className="page-activity__empty">This agent is read-only for your role.</p>
      )}
    </div>
  )
}
