import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiPluginActivation,
  ApiPluginActivationsResponse,
  ApiPluginCuration,
} from "../../lib/api/types"

/**
 * Client for a space's plugin activations.
 *
 * These are space-scoped where /api/plugins is deployment-scoped: the catalog
 * says what exists, an activation says what this space's background runs may
 * use. Reading needs membership; every change needs owner or admin.
 */

function base(spaceId: string): string {
  return `${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/plugin-activations`
}

export function listActivations(
  token: string,
  spaceId: string,
): Promise<ApiPluginActivationsResponse> {
  return requestJson<ApiPluginActivationsResponse>(base(spaceId), {
    headers: authHeaders(token),
  })
}

/** An empty version takes the newest release the space could be pinned to. */
export function activatePlugin(
  token: string,
  spaceId: string,
  pluginName: string,
  version?: string,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(base(spaceId), {
    method: "POST",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ plugin_name: pluginName, version }),
  })
}

/** Moving the pin and suspending are separate decisions, so they are separate calls. */
export function movePin(
  token: string,
  spaceId: string,
  pluginName: string,
  version: string,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(
    `${base(spaceId)}/${encodeURIComponent(pluginName)}`,
    {
      method: "PATCH",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ version }),
    },
  )
}

export function setActivationEnabled(
  token: string,
  spaceId: string,
  pluginName: string,
  enabled: boolean,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(
    `${base(spaceId)}/${encodeURIComponent(pluginName)}`,
    {
      method: "PATCH",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ enabled }),
    },
  )
}

export function setCuration(
  token: string,
  spaceId: string,
  curation: ApiPluginCuration,
): Promise<{ curation: ApiPluginCuration }> {
  return requestJson<{ curation: ApiPluginCuration }>(
    `${getApiBase()}/api/spaces/${encodeURIComponent(spaceId)}/plugin-curation`,
    {
      method: "PUT",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ curation }),
    },
  )
}
