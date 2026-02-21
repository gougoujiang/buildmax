import { cn } from "../lib/cn"
import type { LoginUser } from "../lib/api"
import { getUserInitials } from "../lib/getUserInitials"

interface UserAvatarProps {
  user: LoginUser
  size?: "sm" | "md"
  className?: string
}

export function UserAvatar({ user, size = "md", className }: UserAvatarProps) {
  return (
    <span
      className={cn("user-avatar", size && `user-avatar--${size}`, className)}
      aria-hidden
    >
      {getUserInitials(user)}
    </span>
  )
}
