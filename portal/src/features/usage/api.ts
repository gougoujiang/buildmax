import { getApiBase, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type { ApiUsage } from "../../lib/api/types"

export async function getUsage(token: string): Promise<ApiUsage> {
  return requestJson<ApiUsage>(`${getApiBase()}/api/usage`, {
    headers: authHeaders(token),
  })
}
