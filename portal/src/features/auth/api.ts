import { getApiBase, requestJson } from "../../lib/api/client"
import { jsonHeaders } from "../../lib/api/common"
import { currentAccessToken, currentRefreshToken } from "../../lib/api/session"
import type { LoginResponse, OtpRequestResponse } from "../../lib/api/types"

export async function requestOtp(
  email: string,
  intent: "signup" | "login"
): Promise<OtpRequestResponse> {
  return requestJson<OtpRequestResponse>(`${getApiBase()}/api/otp/request`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ email, intent }),
  })
}

export async function login(email: string, otp: string): Promise<LoginResponse> {
  return requestJson<LoginResponse>(`${getApiBase()}/api/login`, {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({ email, otp, platform: "portal" }),
  })
}

/**
 * Ask the server to revoke this session.
 *
 * Never throws. Logging out has to work when the server is unreachable, and a
 * signed-out user who is still looking at the app because the call failed is a
 * worse outcome than a session row that outlives its client.
 */
export async function revokeSession(): Promise<void> {
  const refreshToken = currentRefreshToken()
  const accessToken = currentAccessToken()
  if (!refreshToken && !accessToken) return
  try {
    await fetch(`${getApiBase()}/api/logout`, {
      method: "POST",
      headers: accessToken
        ? { ...jsonHeaders, Authorization: `Bearer ${accessToken}` }
        : jsonHeaders,
      body: JSON.stringify({ refresh_token: refreshToken ?? "" }),
      // The tab may be closing. Without this the request is cancelled and the
      // session survives a deliberate sign-out.
      keepalive: true,
    })
  } catch {
    // See above.
  }
}
