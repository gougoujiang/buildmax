import type { LoginUser } from "./api"

export function getUserInitials(user: LoginUser): string {
  if (user.name?.trim()) {
    const parts = user.name.trim().split(/\s+/)
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase().slice(0, 2)
    }
    return user.name.trim().slice(0, 2).toUpperCase()
  }
  const local = user.email.split("@")[0]
  return (local.slice(0, 2) || "?").toUpperCase()
}
