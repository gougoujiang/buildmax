import { describe, expect, it } from "vitest"
import { contributionRows, newestInstallable } from "./PluginCatalog"
import type { ApiPluginRelease } from "../../lib/api/types"

function release(version: string, overrides: Partial<ApiPluginRelease> = {}): ApiPluginRelease {
  return {
    plugin_name: "code-review",
    version,
    digest: "sha256:abc",
    object_key: "k",
    size_bytes: 1,
    inspection: {},
    source: {},
    published_by: "u_admin",
    published_at: "1970-01-01T00:00:01Z",
    ...overrides,
  }
}

describe("newestInstallable", () => {
  // This is the release `buildmax plugin install` would take, so the page must
  // name the same one the command would.
  it("takes the newest stable release", () => {
    const got = newestInstallable([release("1.0.0"), release("1.10.0"), release("1.9.0")])
    expect(got?.version).toBe("1.10.0")
  })

  it("skips prereleases, which the default install also skips", () => {
    expect(newestInstallable([release("1.0.0"), release("2.0.0-rc.1")])?.version).toBe("1.0.0")
  })

  it("skips withdrawn releases", () => {
    const got = newestInstallable([release("1.0.0"), release("1.1.0", { yanked_at: "1970-01-01T00:00:01Z" })])
    expect(got?.version).toBe("1.0.0")
  })

  it("has nothing to offer when every release was withdrawn", () => {
    expect(newestInstallable([release("1.0.0", { yanked_at: "1970-01-01T00:00:01Z" })])).toBeNull()
  })

  it("skips a version it cannot order", () => {
    expect(newestInstallable([release("not-a-version"), release("1.0.0")])?.version).toBe("1.0.0")
  })
})

describe("contributionRows", () => {
  it("lists one line per kind", () => {
    const rows = contributionRows(
      release("1.0.0", {
        inspection: {
          skills: ["review"],
          subagents: [{ name: "reviewer" }],
          mcp: [{ id: "github", transport: "stdio" }],
        },
      }),
    )
    expect(rows).toEqual([
      "Skills: review",
      "Subagents: reviewer",
      "MCP servers: github (stdio)",
    ])
  })

  it("names an empty release rather than showing nothing", () => {
    expect(contributionRows(release("1.0.0"))).toEqual([
      "Contributes nothing this build recognises",
    ])
  })
})
