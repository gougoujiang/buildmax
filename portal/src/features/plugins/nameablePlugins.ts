import type {
  ApiPlugin,
  ApiPluginActivationsResponse,
} from "../../lib/api/types"

/**
 * nameablePlugins returns the catalog plugin names an agent in this team may
 * name, sorted. It mirrors the server's rule (service/agent resolvePlugins):
 *
 *   - A curated team may name only its enabled activations.
 *   - An open team may name any non-archived catalog plugin; naming one
 *     activates it automatically.
 *
 * The frontend only offers valid choices; the server checks again and is the
 * authority. Either input may be null (a request failed, or the deployment has
 * no Marketplace), which contributes nothing rather than throwing.
 */
export function nameablePlugins(
  activations: ApiPluginActivationsResponse | null,
  catalog: ApiPlugin[] | null,
): string[] {
  const names = new Set<string>()
  for (const activation of activations?.activations ?? []) {
    if (activation.enabled) names.add(activation.plugin_name)
  }
  // Curated is the safe default: an unknown mode must not widen the choices to
  // the whole catalog.
  if (activations?.curation === "open") {
    for (const plugin of catalog ?? []) {
      if (!plugin.archived_at) names.add(plugin.name)
    }
  }
  return [...names].sort()
}
