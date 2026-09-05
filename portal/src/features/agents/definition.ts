import type { FormModalFieldConfig } from "@buildmax/gui"
import type { ApiSecretConsumption } from "../../lib/api/types"
import {
  AGENT_SANDBOX_FILESYSTEM_TIER_OPTIONS,
  AGENT_SANDBOX_NETWORK_TIER_OPTIONS,
} from "../../lib/sandboxTiers"

// The empty-value option for the model select: no name means the run uses the
// deployment's default model.
export const DEPLOYMENT_DEFAULT_MODEL_OPTION = { value: "", label: "Deployment default" }

// The scalar agent fields (everything but the plugin and secret sub-editors),
// the single source both the create dialog and the inline detail-page editor
// render from. `group` keys are used only by the create dialog's tabbed
// FormModal; the inline editor ignores them. The model select's options are a
// placeholder here; call agentFields(models) to fill them from the catalog.
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
    key: "model",
    label: "Model",
    type: "select",
    optional: true,
    // Options are the deployment default plus whatever the catalog lists;
    // agentFields fills them in, since they are not known at module load.
    options: [DEPLOYMENT_DEFAULT_MODEL_OPTION],
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

// agentFields returns the field set with the model select's options filled from
// the catalog: the deployment default first, then every model the deployment
// offers. Both editors call it with the models they fetched, so a deployment
// with no catalog (or one still loading) shows only the default.
export function agentFields(models: string[]): FormModalFieldConfig[] {
  const options = [
    DEPLOYMENT_DEFAULT_MODEL_OPTION,
    ...models.map((name) => ({ value: name, label: name })),
  ]
  return AGENT_FIELDS.map((field) => (field.key === "model" ? { ...field, options } : field))
}

export interface AgentDefinitionInput {
  name: string
  description?: string
  instructions?: string
  model?: string
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
    model: values.model || undefined,
    plugins,
    sandbox_network_tier: values.sandbox_network_tier || undefined,
    sandbox_filesystem_tier: values.sandbox_filesystem_tier || undefined,
    secret_consumption: normalizeConsumption(consumption),
  }
}
