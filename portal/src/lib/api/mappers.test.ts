import { describe, expect, it } from "vitest"
import type { ApiAgent } from "./types"
import { apiAgentToAgent } from "./mappers"

describe("apiAgentToAgent", () => {
  it("carries the declared sandbox tiers through", () => {
    const api: ApiAgent = {
      id: "a_1",
      user_id: "u_1",
      team_id: "tm_1",
      name: "Reviewer",
      description: "",
      instructions: "",
      sandbox_network_tier: "registries",
      sandbox_filesystem_tier: "workspace_plus_shared_read",
      revision: 1,
      created_at: "2026-08-01T00:00:00Z",
    }
    const agent = apiAgentToAgent(api)
    expect(agent.sandboxNetworkTier).toBe("registries")
    expect(agent.sandboxFilesystemTier).toBe("workspace_plus_shared_read")
  })

  it("leaves the tiers undefined when the agent declares neither", () => {
    const api: ApiAgent = {
      id: "a_1",
      user_id: "u_1",
      team_id: "tm_1",
      name: "Reviewer",
      description: "",
      instructions: "",
      revision: 1,
      created_at: "2026-08-01T00:00:00Z",
    }
    const agent = apiAgentToAgent(api)
    expect(agent.sandboxNetworkTier).toBeUndefined()
    expect(agent.sandboxFilesystemTier).toBeUndefined()
  })
})
