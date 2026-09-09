import { describe, expect, it } from "vitest"
import { findParentId } from "./explore"
import type { ExploreNode } from "./types"

const tree: ExploreNode = {
  id: "root-node",
  name: "root",
  type: "folder",
  children: [
    {
      id: "docs",
      name: "docs",
      type: "folder",
      children: [
        { id: "notes.md", name: "notes.md", type: "file" },
        {
          id: "nested",
          name: "nested",
          type: "folder",
          children: [{ id: "deep.txt", name: "deep.txt", type: "file" }],
        },
      ],
    },
    { id: "readme.md", name: "readme.md", type: "file" },
  ],
}

describe("findParentId", () => {
  it("returns the sentinel root id for a top-level node's parent", () => {
    expect(findParentId(tree, "docs")).toBe(".")
    expect(findParentId(tree, "readme.md")).toBe(".")
  })

  it("returns the containing folder's real id for a nested node", () => {
    expect(findParentId(tree, "notes.md")).toBe("docs")
    expect(findParentId(tree, "nested")).toBe("docs")
    expect(findParentId(tree, "deep.txt")).toBe("nested")
  })

  it("returns null for the root itself", () => {
    expect(findParentId(tree, "root-node")).toBeNull()
  })

  it("returns null for an id that is not in the tree", () => {
    expect(findParentId(tree, "missing")).toBeNull()
  })
})
