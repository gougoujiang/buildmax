/**
 * Permission state is separate from resource state (see
 * docs/design/portal-state-and-permission-feedback.md#permission-model).
 * `unknown` and `failed` must never be treated as `denied`: only `denied`
 * means the server evaluated the check and refused it.
 */
export type PermissionState = "unknown" | "allowed" | "denied" | "failed"

export interface DerivePermissionStateInput {
  /** The permission or role lookup this depends on is in flight, or has not started. */
  loading: boolean
  /** The lookup itself failed (network/server error), as opposed to resolving to a refusal. */
  lookupFailed: boolean
  /** The resolved permission. Ignored while loading or failed. */
  allowed: boolean
}

/** Pure derivation of PermissionState from a permission or role lookup's request primitives. */
export function derivePermissionState(input: DerivePermissionStateInput): PermissionState {
  if (input.loading) return "unknown"
  if (input.lookupFailed) return "failed"
  return input.allowed ? "allowed" : "denied"
}

/** True only once the permission is known to allow the action — never for unknown or failed. */
export function isAllowed(state: PermissionState): boolean {
  return state === "allowed"
}
