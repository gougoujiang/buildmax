/**
 * A mutation's local lifecycle (see
 * docs/design/portal-state-and-permission-feedback.md#mutation-feedback):
 * the initiating control shows progress, refuses a duplicate submission
 * while one is in flight, and stays associated with its own result rather
 * than a page-wide status.
 */
export type MutationState = "idle" | "submitting" | "succeeded" | "failed"

export interface DeriveMutationStateInput {
  submitting: boolean
  /** The most recent attempt's outcome, once settled. Ignored while submitting. */
  succeeded: boolean
  failed: boolean
}

/** Pure derivation of MutationState from a mutation's request primitives. */
export function deriveMutationState(input: DeriveMutationStateInput): MutationState {
  if (input.submitting) return "submitting"
  if (input.failed) return "failed"
  if (input.succeeded) return "succeeded"
  return "idle"
}

/** True only while submitting: the one state in which a duplicate submission must be refused. */
export function isSubmitting(state: MutationState): boolean {
  return state === "submitting"
}
