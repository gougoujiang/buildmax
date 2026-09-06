import { describe, expect, it } from "vitest"
import { pageWindow } from "./pagination"

describe("pageWindow", () => {
  it("describes the first of several pages", () => {
    expect(pageWindow(0, 50, 130)).toEqual({
      from: 1,
      to: 50,
      hasPrev: false,
      hasNext: true,
      prevOffset: 0,
      nextOffset: 50,
    })
  })

  it("describes a middle page", () => {
    const w = pageWindow(50, 50, 130)
    expect([w.from, w.to, w.hasPrev, w.hasNext]).toEqual([51, 100, true, true])
  })

  it("describes a short last page", () => {
    const w = pageWindow(100, 50, 130)
    expect([w.from, w.to, w.hasPrev, w.hasNext]).toEqual([101, 130, true, false])
  })

  it("has no next when the total fits one page", () => {
    const w = pageWindow(0, 50, 30)
    expect([w.from, w.to, w.hasPrev, w.hasNext]).toEqual([1, 30, false, false])
  })

  it("reports an empty window when the offset is past the end", () => {
    const w = pageWindow(200, 50, 130)
    expect([w.from, w.to, w.hasNext]).toEqual([0, 200, false])
    expect(w.prevOffset).toBe(150)
  })
})
