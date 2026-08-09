import { TreePanel } from "../../../components/TreePanel"
import type { ExploreNode } from "../../../lib/types"

interface FileTreeProps {
  tree: ExploreNode | null
  treeLoading: boolean
  treeError: string | null
  expandedIds: Set<string>
  selectedFolderId: string
  onToggle: (id: string) => void
  onSelectFolder: (id: string) => void
}

export function FileTree({
  tree,
  treeLoading,
  treeError,
  expandedIds,
  selectedFolderId,
  onToggle,
  onSelectFolder,
}: FileTreeProps) {
  return (
    <aside className="page-explore__tree-panel" aria-label="Directory tree">
      <div className="explore-tree">
        {treeLoading && <p className="explore-tree__loading">Loading…</p>}
        {treeError && <p className="explore-tree__error">{treeError}</p>}
        {tree && tree.type === "folder" && (
          <ul className="explore-tree__list" role="tree" aria-label="Folder tree">
            <TreePanel
              node={tree}
              depth={0}
              expandedIds={expandedIds}
              selectedFolderId={selectedFolderId}
              onToggle={onToggle}
              onSelectFolder={onSelectFolder}
            />
          </ul>
        )}
      </div>
    </aside>
  )
}
