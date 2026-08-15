import { afterEach, describe, expect, it, vi } from "vitest"
import { getApiBase } from "./client"

// The published image ships one bundle for every deployment, so this precedence
// is what makes the image usable at all. A regression here is invisible in dev
// (where VITE_API_BASE or the default answers) and total in a container.
describe("getApiBase", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
  })

  it("falls back to the local server when nothing is configured", () => {
    vi.stubGlobal("window", {})
    expect(getApiBase()).toBe("http://localhost:5678")
  })

  it("uses the build-time value when there is no runtime config", () => {
    vi.stubGlobal("window", { __BUILDMAX_CONFIG__: {} })
    vi.stubEnv("VITE_API_BASE", "http://build.example.com")
    expect(getApiBase()).toBe("http://build.example.com")
  })

  it("prefers the runtime config over the build-time value", () => {
    vi.stubGlobal("window", { __BUILDMAX_CONFIG__: { apiBase: "https://runtime.example.com" } })
    vi.stubEnv("VITE_API_BASE", "http://build.example.com")
    expect(getApiBase()).toBe("https://runtime.example.com")
  })

  it("ignores an empty runtime value, which is what an unset env var writes", () => {
    vi.stubGlobal("window", { __BUILDMAX_CONFIG__: { apiBase: "" } })
    vi.stubEnv("VITE_API_BASE", "http://build.example.com")
    expect(getApiBase()).toBe("http://build.example.com")
  })

  it("trims trailing slashes so request paths do not double up", () => {
    vi.stubGlobal("window", { __BUILDMAX_CONFIG__: { apiBase: "https://api.example.com/" } })
    expect(getApiBase()).toBe("https://api.example.com")
  })

  it('turns "/" into a same-origin base, for a reverse proxy in front of both', () => {
    vi.stubGlobal("window", { __BUILDMAX_CONFIG__: { apiBase: "/" } })
    expect(getApiBase()).toBe("")
  })
})
