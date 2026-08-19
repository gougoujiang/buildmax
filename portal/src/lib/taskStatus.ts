import type { Task } from "./types"

/**
 * A task that can still be stopped: its run has not reached a terminal status.
 *
 * Scheduled runs arrive here as "running" — from the outside a run waiting for
 * a worker and one executing are the same thing, work in flight that a person
 * may want to stop.
 */
export function taskIsStoppable(status: Task["status"]): boolean {
  return status === "pending" || status === "running"
}

/**
 * A task whose run is over, so running it again is a thing that can be asked
 * for. Succeeded runs are included: repeating work that worked is normal, and
 * the server is what refuses the cases it has to.
 */
export function taskIsRetryable(status: Task["status"]): boolean {
  return !taskIsStoppable(status)
}

export function taskStatusIcon(status: Task["status"]): string {
  switch (status) {
    case "success":
      return "\u2705"
    case "running":
      return "\u23f3"
    case "failed":
      return "\u274c"
    case "canceled":
      return "\u26d4"
    case "pending":
      return "\ud83d\udd50"
  }
}
