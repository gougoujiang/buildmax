import { describe, expect, it } from "vitest"
import { agentFields, buildAgentDefinition } from "./definition"

describe("buildAgentDefinition", () => {
  it("sends the chosen model", () => {
    const def = buildAgentDefinition({ name: "picker", model: "Fast" }, [], { env: [] })
    expect(def?.model).toBe("Fast")
  })

  it("omits the model when the deployment default is chosen", () => {
    const def = buildAgentDefinition({ name: "picker", model: "" }, [], { env: [] })
    expect(def?.model).toBeUndefined()
  })
})

describe("agentFields", () => {
  it("offers the deployment default first, then the catalog models", () => {
    const field = agentFields(["Fast", "Deep"]).find((f) => f.key === "model")
    expect(field?.options).toEqual([
      { value: "", label: "Deployment default" },
      { value: "Fast", label: "Fast" },
      { value: "Deep", label: "Deep" },
    ])
  })

  it("offers only the deployment default when the catalog is empty", () => {
    const field = agentFields([]).find((f) => f.key === "model")
    expect(field?.options).toEqual([{ value: "", label: "Deployment default" }])
  })
})
