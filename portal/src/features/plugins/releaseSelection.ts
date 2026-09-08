import type { ApiPluginRelease } from "../../lib/api/types"

/**
 * newestInstallable is what `buildmax plugin install` would take by default.
 *
 * Withdrawn releases are skipped, and so is anything that will not order —
 * publishing rejects those, so one here came from a build that did not.
 */
export function newestInstallable(releases: ApiPluginRelease[]): ApiPluginRelease | null {
  let best: ApiPluginRelease | null = null
  let bestKey: number[] | null = null
  for (const release of releases) {
    if (release.yanked_at) continue
    const key = releaseOrder(release.version)
    if (!key) continue
    if (!bestKey || compareOrder(key, bestKey) > 0) {
      best = release
      bestKey = key
    }
  }
  return best
}

/** releaseOrder parses a stable release, or null for a prerelease or garbage. */
function releaseOrder(version: string): number[] | null {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(version)
  if (!match) return null
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

function compareOrder(a: number[], b: number[]): number {
  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) return a[i] - b[i]
  }
  return 0
}
