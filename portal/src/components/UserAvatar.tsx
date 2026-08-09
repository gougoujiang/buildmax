import type { LoginUser } from "../lib/api"
import { Avatar, getInitials } from "@buildmax/gui"

interface UserAvatarProps {
  user: LoginUser
  size?: "sm" | "md"
  className?: string
}

export function UserAvatar({ user, size = "md", className }: UserAvatarProps) {
  const fallback = user.email.split("@")[0]

  return (
    <Avatar
      label={getInitials(user.name, fallback)}
      size={size}
      className={className}
    />
  )
}

interface AgentAvatarProps {
  size?: "sm" | "md"
  className?: string
}

export function AgentAvatar({ size = "sm", className }: AgentAvatarProps) {
  return <Avatar label="A" size={size} className={className} />
}
