import { getApiBase, requestText } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"

/**
 * Read one file a task run left in its output directory.
 *
 * This is the compatibility surface: a run output is addressed by its run and a
 * relative path, not by an id. Durable files a team keeps are artifacts and
 * live in `features/artifacts` — see docs/design/unified-artifacts.md.
 */
export async function getRunOutputContent(
  teamId: string,
  taskRunId: string,
  token: string,
  path?: string
): Promise<string> {
  let url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/task-runs/${encodeURIComponent(taskRunId)}/artifacts/content`
  if (path) {
    url += `?path=${encodeURIComponent(path)}`
  }
  return requestText(url, { headers: authHeaders(token) })
}
