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
