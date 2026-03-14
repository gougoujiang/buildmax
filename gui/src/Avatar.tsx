function cn(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(" ")
}

export interface AvatarProps {
  label: string
  size?: "sm" | "md" | "lg"
  className?: string
}

export function getInitials(name?: string, fallback?: string): string {
  if (name?.trim()) {
    const parts = name.trim().split(/\s+/)
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase().slice(0, 2)
    }
    return name.trim().slice(0, 2).toUpperCase()
  }

  if (fallback?.trim()) {
    return fallback.trim().slice(0, 2).toUpperCase()
  }

  return "?"
}

export function Avatar({ label, size = "md", className }: AvatarProps) {
  return (
    <span
      className={cn("bm-avatar", size && `bm-avatar--${size}`, className)}
      aria-hidden
    >
      {label}
    </span>
  )
}
