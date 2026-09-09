import { describe, expect, it } from "vitest"
import { derivePermissionState, isAllowed } from "./permissionState"

describe("derivePermissionState", () => {
  it("is unknown while the lookup is loading, regardless of a stale allowed/failed value", () => {
    expect(derivePermissionState({ loading: true, lookupFailed: false, allowed: true })).toBe("unknown")
    expect(derivePermissionState({ loading: true, lookupFailed: true, allowed: false })).toBe("unknown")
  })

  it("is failed when the lookup errored, not denied", () => {
    // A lookup failure and a real refusal must stay distinguishable, or a
    // page cannot tell "explain and retry" from "this role cannot do that".
    expect(derivePermissionState({ loading: false, lookupFailed: true, allowed: false })).toBe("failed")
  })

  it("is allowed or denied once the lookup has resolved", () => {
    expect(derivePermissionState({ loading: false, lookupFailed: false, allowed: true })).toBe("allowed")
    expect(derivePermissionState({ loading: false, lookupFailed: false, allowed: false })).toBe("denied")
  })
})

describe("isAllowed", () => {
  it("is true only for allowed, never for unknown or failed", () => {
    expect(isAllowed("allowed")).toBe(true)
    expect(isAllowed("unknown")).toBe(false)
    expect(isAllowed("failed")).toBe(false)
    expect(isAllowed("denied")).toBe(false)
  })
})
