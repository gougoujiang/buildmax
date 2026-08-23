import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders, jsonHeaders } from "../../lib/api/common"
import type {
  ApiPluginActivation,
  ApiPluginActivationsResponse,
  ApiPluginCuration,
} from "../../lib/api/types"

/**
 * Client for a team's plugin activations.
 *
 * These are team-scoped where /api/plugins is deployment-scoped: the catalog
 * says what exists, an activation says what this team's background runs may
 * use. Reading needs membership; every change needs owner or admin.
 */

function base(teamId: string): string {
  return `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/plugin-activations`
}

export function listActivations(
  token: string,
  teamId: string,
): Promise<ApiPluginActivationsResponse> {
  return requestJson<ApiPluginActivationsResponse>(base(teamId), {
    headers: authHeaders(token),
  })
}

/** An empty version takes the newest release the team could be pinned to. */
export function activatePlugin(
  token: string,
  teamId: string,
  pluginName: string,
  version?: string,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(base(teamId), {
    method: "POST",
    headers: { ...authHeaders(token), ...jsonHeaders },
    body: JSON.stringify({ plugin_name: pluginName, version }),
  })
}

/** Moving the pin and suspending are separate decisions, so they are separate calls. */
export function movePin(
  token: string,
  teamId: string,
  pluginName: string,
  version: string,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(
    `${base(teamId)}/${encodeURIComponent(pluginName)}`,
    {
      method: "PATCH",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ version }),
    },
  )
}

export function setActivationEnabled(
  token: string,
  teamId: string,
  pluginName: string,
  enabled: boolean,
): Promise<ApiPluginActivation> {
  return requestJson<ApiPluginActivation>(
    `${base(teamId)}/${encodeURIComponent(pluginName)}`,
    {
      method: "PATCH",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ enabled }),
    },
  )
}

export function setCuration(
  token: string,
  teamId: string,
  curation: ApiPluginCuration,
): Promise<{ curation: ApiPluginCuration }> {
  return requestJson<{ curation: ApiPluginCuration }>(
    `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/plugin-curation`,
    {
      method: "PUT",
      headers: { ...authHeaders(token), ...jsonHeaders },
      body: JSON.stringify({ curation }),
    },
  )
}
