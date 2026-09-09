import { describe, expect, it } from "vitest"
import { ApiRequestError } from "../lib/api/client"
import { classifyError, deriveResourceState } from "./resourceState"

const notEmpty = () => false
const isEmptyList = (data: unknown[]) => data.length === 0

describe("classifyError", () => {
  it("maps a 403 to forbidden", () => {
    expect(classifyError(new ApiRequestError("no", 403))).toEqual({ kind: "forbidden", message: "no" })
  })

  it("maps a 404 to notFound", () => {
    expect(classifyError(new ApiRequestError("gone", 404))).toEqual({ kind: "notFound", message: "gone" })
  })

  it("maps any other status to error rather than inventing meaning for it", () => {
    expect(classifyError(new ApiRequestError("boom", 500))).toEqual({ kind: "error", message: "boom" })
  })

  it("maps a plain error and a non-error throw to error, using the fallback when there is no message", () => {
    expect(classifyError(new Error("network down"))).toEqual({ kind: "error", message: "network down" })
    expect(classifyError("nope", "Request failed")).toEqual({ kind: "error", message: "Request failed" })
  })
})

describe("deriveResourceState", () => {
  it("is loading on first fetch, with no prior data to show", () => {
    const got = deriveResourceState({ loading: true, data: null, error: null, isEmpty: notEmpty })
    expect(got).toEqual({ kind: "loading" })
  })

  it("is refreshing, not loading, when a fetch is in flight but prior data exists", () => {
    const got = deriveResourceState({ loading: true, data: [1], error: null, isEmpty: isEmptyList })
    expect(got).toEqual({ kind: "refreshing", data: [1] })
  })

  it("is ready when data is present and not empty", () => {
    const got = deriveResourceState({ loading: false, data: [1], error: null, isEmpty: isEmptyList })
    expect(got).toEqual({ kind: "ready", data: [1] })
  })

  it("is readyEmpty when the request succeeded with no data, not error", () => {
    const got = deriveResourceState({ loading: false, data: [], error: null, isEmpty: isEmptyList })
    expect(got.kind).toBe("readyEmpty")
  })

  it("is readyEmpty rather than error when there is no data and no error at all", () => {
    // e.g. a disabled fetch, or a resource key that has never been requested.
    const got = deriveResourceState({ loading: false, data: null, error: null, isEmpty: notEmpty })
    expect(got.kind).toBe("readyEmpty")
  })

  it("is error, not stale, when a transient failure has no prior data to fall back on", () => {
    const error = { kind: "error" as const, message: "network down" }
    const got = deriveResourceState({ loading: false, data: null, error, isEmpty: notEmpty })
    expect(got).toEqual({ kind: "error", error })
  })

  it("is stale when a transient failure follows successfully loaded data for the same key", () => {
    const error = { kind: "error" as const, message: "network down" }
    const got = deriveResourceState({ loading: false, data: [1], error, isEmpty: isEmptyList })
    expect(got).toEqual({ kind: "stale", data: [1], error })
  })

  it("is forbidden even when prior data exists, overriding stale", () => {
    // A permission change on refresh must not be presented as "old data, retry available".
    const error = { kind: "forbidden" as const, message: "denied" }
    const got = deriveResourceState({ loading: false, data: [1], error, isEmpty: isEmptyList })
    expect(got).toEqual({ kind: "forbidden", error })
  })

  it("is notFound even when prior data exists, overriding stale", () => {
    const error = { kind: "notFound" as const, message: "gone" }
    const got = deriveResourceState({ loading: false, data: [1], error, isEmpty: isEmptyList })
    expect(got).toEqual({ kind: "notFound", error })
  })

  it("never derives readyEmpty and error for the same input", () => {
    for (const error of [
      { kind: "error" as const, message: "m" },
      { kind: "forbidden" as const, message: "m" },
      { kind: "notFound" as const, message: "m" },
    ]) {
      const got = deriveResourceState({ loading: false, data: null, error, isEmpty: notEmpty })
      expect(got.kind).not.toBe("readyEmpty")
    }
  })
})
