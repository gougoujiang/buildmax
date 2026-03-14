export const jsonHeaders = { "Content-Type": "application/json" }

export function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` }
}
