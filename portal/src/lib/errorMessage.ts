/**
 * Normalize unknown errors to a user-facing string.
 */
export function getErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}
