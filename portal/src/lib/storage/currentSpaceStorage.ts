const SPACE_KEY = "buildmax_current_space"

export function getStoredCurrentSpaceId(): string | null {
  if (typeof window === "undefined") return null
  try {
    const value = localStorage.getItem(SPACE_KEY)
    return value && value.trim() !== "" ? value : null
  } catch {
    return null
  }
}

export function setStoredCurrentSpaceId(spaceId: string | null): void {
  if (typeof window === "undefined") return
  try {
    if (spaceId && spaceId.trim() !== "") {
      localStorage.setItem(SPACE_KEY, spaceId)
      return
    }
    localStorage.removeItem(SPACE_KEY)
  } catch {
    // Storage may be unavailable in private or embedded browser contexts.
  }
}

export function clearStoredCurrentSpaceId(): void {
  setStoredCurrentSpaceId(null)
}
