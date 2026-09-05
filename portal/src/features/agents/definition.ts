import type { FormModalFieldConfig } from "@buildmax/gui"
import type { ApiSecretConsumption } from "../../lib/api/types"
import {
  AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
} from "../../lib/sandboxTiers"

// The scalar agent fields (everything but the plugin and secret sub-editors),
// the single source both the create dialog and the inline detail-page editor
// render from. `group` keys are used only by the create dialog's tabbed
// FormModal; the inline editor ignores them.
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

export interface AgentDefinitionInput {
  name: string
  description?: string
  instructions?: string
  plugins?: string[]
  sandbox_network_tier?: string
  sandbox_filesystem_tier?: string
  secret_consumption?: ApiSecretConsumption
}

/**
 * normalizeConsumption drops incomplete grants -- a row with no secret chosen,
 * or a selected item with no variable name -- so a half-filled row does not
 * reach the API. An empty result is sent as an empty object, which clears the
 * agent's consumption (create and PATCH both replace the whole definition).
 */
export function normalizeConsumption(c: ApiSecretConsumption): ApiSecretConsumption {
  const env = (c.env ?? []).filter((g) => {
    if (!g.secret) return false
    if (g.item) return Boolean(g.env_name)
    return true
  })
  return { env }
}

/**
 * buildAgentDefinition assembles the create/update body from the scalar field
 * values plus the two sub-editors' state, so the trimming and normalization
 * rules live in one place. Returns null when the required name is empty, which
 * a caller treats as "do not submit". `plugins` is always sent as the array
 * (empty clears it) because both create and PATCH replace the whole definition.
 */
export function buildAgentDefinition(
  values: Record<string, string>,
  plugins: string[],
  consumption: ApiSecretConsumption,
): AgentDefinitionInput | null {
  const name = values.name?.trim()
  if (!name) return null
  return {
    name,
    description: values.description?.trim() || undefined,
    instructions: values.instructions?.trim() || undefined,
    plugins,
    sandbox_network_tier: values.sandbox_network_tier || undefined,
    sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
    secret_consumption: normalizeConsumption(consumption),
  }
}
