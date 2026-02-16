import { useState, useCallback, useRef, useEffect } from "react"
import type { ExploreNode } from "../lib/types"
import { uploadFiles, getFileTree, getFileContent } from "../lib/api"
import { useAuth } from "../contexts/AuthContext"

interface ExplorePageProps {
  workspaceId: string
}

function isFolder(node: ExploreNode): node is ExploreNode & { type: "folder" } {
  return node.type === "folder"
}

function findNodeById(node: ExploreNode, id: string): ExploreNode | undefined {
  if (node.id === id) return node
  if (node.type === "folder") {
    for (const child of node.children ?? []) {
      const found = findNodeById(child, id)
      if (found) return found
    }
  }
  return undefined
}

function getChildren(root: ExploreNode, folderId: string): ExploreNode[] {
  if (folderId === "" || folderId === ".") {
    return root.type === "folder" ? root.children ?? [] : []
  }
  const node = findNodeById(root, folderId)
  return node?.type === "folder" ? node.children ?? [] : []
}

/** Single folder row in the tree; children rendered in a nested <ul> */
function TreePanel({
  node,
  depth,
  expandedIds,
  selectedFolderId,
  onToggle,
  onSelectFolder,
}: {
  node: ExploreNode
  depth: number
  expandedIds: Set<string>
  selectedFolderId: string
  onToggle: (id: string) => void
  onSelectFolder: (id: string) => void
}) {
  if (node.type !== "folder") return null

  const isExpanded = expandedIds.has(node.id)
  const isSelected = selectedFolderId === node.id
  const folderChildren = (node.children ?? []).filter(isFolder)

  return (
    <li className="explore-tree__item" role="treeitem" style={{ paddingLeft: `${depth * 1.25}rem` }}>
      <button
        type="button"
        className={`explore-tree__row ${isSelected ? "explore-tree__row--selected" : ""}`}
        onClick={() => {
          onSelectFolder(node.id)
          if (folderChildren.length > 0) onToggle(node.id)
        }}
        aria-expanded={folderChildren.length > 0 ? isExpanded : undefined}
      >
        {folderChildren.length > 0 ? (
          <span className="explore-tree__icon" aria-hidden>
            {isExpanded ? "▼" : "▶"}
          </span>
        ) : (
          <span className="explore-tree__icon explore-tree__icon--empty" aria-hidden />
        )}
        <span className="explore-tree__label">{node.name}</span>
      </button>
      {isExpanded && folderChildren.length > 0 && (
        <ul className="explore-tree__list" role="group">
          {folderChildren.map((child) => (
            <TreePanel
              key={child.id}
              node={child}
              depth={depth + 1}
              expandedIds={expandedIds}
              selectedFolderId={selectedFolderId}
              onToggle={onToggle}
              onSelectFolder={onSelectFolder}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

export function ExplorePage({ workspaceId }: ExplorePageProps) {
  const { token } = useAuth()
  const [tree, setTree] = useState<ExploreNode | null>(null)
  const [treeLoading, setTreeLoading] = useState(true)
  const [treeError, setTreeError] = useState<string | null>(null)
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set(["."]))
  const [selectedFolderId, setSelectedFolderId] = useState(".")
  const [selectedFileId, setSelectedFileId] = useState<string | null>(null)
  const [fileContent, setFileContent] = useState<string | null>(null)
  const [fileLoading, setFileLoading] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadMsg, setUploadMsg] = useState<{ text: string; isError: boolean } | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const folderInputRef = useRef<HTMLInputElement>(null)

  const fetchTree = useCallback(async () => {
    if (!token) return
    setTreeLoading(true)
    setTreeError(null)
    try {
      const data = await getFileTree(workspaceId, token)
      setTree(data)
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load files"
      setTreeError(msg)
    } finally {
      setTreeLoading(false)
    }
  }, [workspaceId, token])

  useEffect(() => {
    fetchTree()
  }, [fetchTree])

  const handleToggle = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleFileSelect = useCallback(
    async (node: ExploreNode) => {
      if (node.type !== "file" || !token) return
      setSelectedFileId(node.id)
      setFileContent(null)
      setFileLoading(true)
      try {
        const content = await getFileContent(workspaceId, node.id, token)
        setFileContent(content)
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to load file"
        setFileContent(`Error: ${msg}`)
      } finally {
        setFileLoading(false)
      }
    },
    [workspaceId, token]
  )

  const handleUpload = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files
      if (!selected || selected.length === 0) return
      if (selected.length > 10) {
        setUploadMsg({ text: "Too many files (max 10)", isError: true })
        if (fileInputRef.current) fileInputRef.current.value = ""
        return
      }
      if (!token) {
        setUploadMsg({ text: "Not authenticated", isError: true })
        return
      }
      setUploading(true)
      setUploadMsg(null)
      try {
        const files = Array.from(selected)
        const res = await uploadFiles(workspaceId, files, token)
        setUploadMsg({ text: `Uploaded ${res.uploaded.length} file(s)`, isError: false })
        await fetchTree()
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Upload failed"
        setUploadMsg({ text: msg, isError: true })
      } finally {
        setUploading(false)
        if (fileInputRef.current) fileInputRef.current.value = ""
      }
    },
    [workspaceId, token, fetchTree]
  )

  const handleFolderUpload = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files
      if (!selected || selected.length === 0) return
      if (!token) {
        setUploadMsg({ text: "Not authenticated", isError: true })
        return
      }
      setUploading(true)
      setUploadMsg(null)
      try {
        const files = Array.from(selected)
        const paths = files.map((f) => f.webkitRelativePath)
        const res = await uploadFiles(workspaceId, files, token, paths)
        setUploadMsg({ text: `Uploaded ${res.uploaded.length} file(s)`, isError: false })
        await fetchTree()
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Upload failed"
        setUploadMsg({ text: msg, isError: true })
      } finally {
        setUploading(false)
        if (folderInputRef.current) folderInputRef.current.value = ""
      }
    },
    [workspaceId, token, fetchTree]
  )

  const children = tree ? getChildren(tree, selectedFolderId) : []
  const selectedFolderNode = tree ? findNodeById(tree, selectedFolderId) : undefined
  const folderName =
    selectedFolderId === "."
      ? "Workspace"
      : selectedFolderNode?.name ?? "—"

  const selectedFileName = tree && selectedFileId
    ? findNodeById(tree, selectedFileId)?.name ?? null
    : null

  return (
    <div className="page-explore">
      <h1 className="page-explore__title">Explore</h1>
      <p className="page-explore__subtitle">
        Browse workspace structure. Select a folder in the tree, then open a file to view its content.
      </p>

      <div className="page-explore__upload-bar">
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="page-explore__file-input"
          onChange={handleUpload}
        />
        <input
          ref={folderInputRef}
          type="file"
          className="page-explore__file-input"
          onChange={handleFolderUpload}
          {...{ webkitdirectory: "", directory: "" }}
        />
        <button
          type="button"
          className="page-explore__upload-btn"
          disabled={uploading}
          onClick={() => fileInputRef.current?.click()}
        >
          {uploading ? "Uploading…" : "Upload Files"}
        </button>
        <button
          type="button"
          className="page-explore__upload-btn"
          disabled={uploading}
          onClick={() => folderInputRef.current?.click()}
        >
          {uploading ? "Uploading…" : "Upload Folder"}
        </button>
        {uploadMsg && (
          <span
            className={
              "page-explore__upload-msg" +
              (uploadMsg.isError ? " page-explore__upload-msg--error" : "")
            }
          >
            {uploadMsg.text}
          </span>
        )}
      </div>

      <div className="page-explore__panels">
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
                  onToggle={handleToggle}
                  onSelectFolder={setSelectedFolderId}
                />
              </ul>
            )}
          </div>
        </aside>

        <div className="page-explore__content-panel">
          <div className="page-explore__file-list-wrap">
            <h2 className="page-explore__content-heading">
              {folderName}
            </h2>
            <ul className="page-explore__list" role="list">
              {children.length === 0 ? (
                <li className="page-explore__empty">(empty)</li>
              ) : (
                children.map((node) => (
                  <li key={node.id} className="page-explore__item">
                    {node.type === "folder" ? (
                      <button
                        type="button"
                        className="page-explore__link"
                        onClick={() => {
                          setSelectedFolderId(node.id)
                          setExpandedIds((prev) => new Set(prev).add(node.id))
                          setSelectedFileId(null)
                          setFileContent(null)
                        }}
                      >
                        <span className="page-explore__icon page-explore__icon--folder" aria-hidden>📁</span>
                        <span className="page-explore__artifact-title">{node.name}</span>
                      </button>
                    ) : (
                      <button
                        type="button"
                        className={`page-explore__link ${selectedFileId === node.id ? "page-explore__link--selected" : ""}`}
                        onClick={() => handleFileSelect(node)}
                      >
                        <span className="page-explore__icon page-explore__icon--file" aria-hidden>📄</span>
                        <span className="page-explore__artifact-title">{node.name}</span>
                      </button>
                    )}
                  </li>
                ))
              )}
            </ul>
          </div>

          {selectedFileId && (
            <section className="page-explore__viewer" aria-label="File content">
              <h3 className="page-explore__viewer-title">{selectedFileName ?? selectedFileId}</h3>
              {fileLoading ? (
                <p className="page-explore__viewer-loading">Loading…</p>
              ) : (
                <pre className="page-explore__viewer-content">{fileContent ?? ""}</pre>
              )}
            </section>
          )}
        </div>
      </div>
    </div>
  )
}
