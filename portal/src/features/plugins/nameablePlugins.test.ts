import { describe, expect, it } from "vitest"
import type {
  ApiPlugin,
  ApiPluginActivation,
  ApiPluginActivationsResponse,
  ApiPluginCuration,
} from "../../lib/api/types"
import { nameablePlugins } from "./nameablePlugins"

function activation(name: string, over: Partial<ApiPluginActivation> = {}): ApiPluginActivation {
  return {
    id: `act_${name}`,
    team_id: "tm_1",
    plugin_name: name,
    version: "1.0.0",
    digest: "",
    enabled: true,
    origin: "curated",
    activated_by: "u1",
    activated_at: "",
    updated_at: "",
    ...over,
  }
}

function activations(
  curation: ApiPluginCuration,
  list: ApiPluginActivation[],
): ApiPluginActivationsResponse {
  return { curation, activations: list }
}

function plugin(name: string, over: Partial<ApiPlugin> = {}): ApiPlugin {
  return {
    name,
    created_by: "u1",
    created_at: "",
    updated_at: "",
    ...over,
  }
}

describe("nameablePlugins", () => {
  it("returns nothing when both inputs are null", () => {
    expect(nameablePlugins(null, null)).toEqual([])
  })

  it("curated: offers only the enabled activations, sorted", () => {
    const acts = activations("curated", [
      activation("zebra"),
      activation("alpha"),
      activation("suspended", { enabled: false }),
    ])
    expect(nameablePlugins(acts, [plugin("catalog-only")])).toEqual(["alpha", "zebra"])
  })

  it("open: adds every non-archived catalog plugin to the activations", () => {
    const acts = activations("open", [activation("active-one")])
    const catalog = [plugin("browser"), plugin("retired", { archived_at: "2026-01-01" })]
    expect(nameablePlugins(acts, catalog)).toEqual(["active-one", "browser"])
  })

  it("open with no catalog fetched still returns the activations", () => {
    const acts = activations("open", [activation("only-activation")])
    expect(nameablePlugins(acts, null)).toEqual(["only-activation"])
  })

  it("dedupes a plugin that is both activated and in the open catalog", () => {
    const acts = activations("open", [activation("shared")])
    expect(nameablePlugins(acts, [plugin("shared")])).toEqual(["shared"])
  })
})
