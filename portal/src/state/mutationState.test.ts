import { describe, expect, it } from "vitest"
import { deriveMutationState, isSubmitting } from "./mutationState"

describe("deriveMutationState", () => {
  it("is idle before any attempt", () => {
    expect(deriveMutationState({ submitting: false, succeeded: false, failed: false })).toBe("idle")
  })

  it("is submitting while in flight, regardless of a stale succeeded/failed value", () => {
    expect(deriveMutationState({ submitting: true, succeeded: true, failed: false })).toBe("submitting")
    expect(deriveMutationState({ submitting: true, succeeded: false, failed: true })).toBe("submitting")
  })

  it("is failed once settled with an error", () => {
    expect(deriveMutationState({ submitting: false, succeeded: false, failed: true })).toBe("failed")
  })

  it("is succeeded once settled without an error", () => {
    expect(deriveMutationState({ submitting: false, succeeded: true, failed: false })).toBe("succeeded")
  })
})

describe("isSubmitting", () => {
  it("is true only for submitting — the one state where a duplicate must be refused", () => {
    expect(isSubmitting("submitting")).toBe(true)
    expect(isSubmitting("idle")).toBe(false)
    expect(isSubmitting("succeeded")).toBe(false)
    expect(isSubmitting("failed")).toBe(false)
  })
})
