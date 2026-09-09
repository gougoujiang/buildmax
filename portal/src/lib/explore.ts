import type { ExploreNode } from "./types"

export function isFolder(node: ExploreNode): node is ExploreNode & { type: "folder" } {
  return node.type === "folder"
}

export function findNodeById(node: ExploreNode, id: string): ExploreNode | undefined {
  if (node.id === id) return node
  if (node.type === "folder") {
    for (const child of node.children ?? []) {
      const found = findNodeById(child, id)
      if (found) return found
    }
  }
  return undefined
}

export function getChildren(root: ExploreNode, folderId: string): ExploreNode[] {
  if (folderId === "" || folderId === ".") {
    return root.type === "folder" ? root.children ?? [] : []
  }
  const node = findNodeById(root, folderId)
  return node?.type === "folder" ? node.children ?? [] : []
}

/**
 * The id of the folder containing `id`, using "." for the root sentinel (the
 * root's own id is opaque and never addressed directly elsewhere). Returns
 * null when `id` names the root itself or is not found — both "there is no
 * parent to go back to".
 */
export function findParentId(root: ExploreNode, id: string): string | null {
  if (root.id === id) return null

  function walk(node: ExploreNode): string | null {
    if (node.type !== "folder") return null
    for (const child of node.children ?? []) {
      if (child.id === id) return node.id === root.id ? "." : node.id
      const found = walk(child)
      if (found !== null) return found
    }
    return null
  }

  return walk(root)
}
