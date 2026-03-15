import type { Task } from "./types"

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
