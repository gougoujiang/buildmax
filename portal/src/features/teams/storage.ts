const TEAM_KEY = "buildmax_current_team"

export function getStoredCurrentTeamId(): string | null {
  if (typeof window === "undefined") return null
  try {
    const value = localStorage.getItem(TEAM_KEY)
    return value && value.trim() !== "" ? value : null
  } catch {
    return null
  }
}

export function setStoredCurrentTeamId(teamId: string | null): void {
  if (typeof window === "undefined") return
  try {
    if (teamId && teamId.trim() !== "") {
      localStorage.setItem(TEAM_KEY, teamId)
      return
    }
    localStorage.removeItem(TEAM_KEY)
  } catch {
    // ignore storage failures
  }
}

export function clearStoredCurrentTeamId(): void {
  setStoredCurrentTeamId(null)
}
