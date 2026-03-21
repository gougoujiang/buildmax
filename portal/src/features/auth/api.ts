import { getApiBase, requestJson } from "../../lib/api/client"
import { jsonHeaders } from "../../lib/api/common"
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
