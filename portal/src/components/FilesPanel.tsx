import { useState, useCallback, useRef } from "react"
import type { ExploreNode } from "../lib/types"
import { getChildren, findNodeById } from "../lib/explore"
import { uploadFiles, getFileTree, getFileContent } from "../lib/api"
import { useAuth } from "../contexts/AuthContext"
import { useFetch } from "../hooks/useFetch"
import { TreePanel } from "./TreePanel"

interface FilesPanelProps {
  workspaceId: string
  /** Optional class name for the root container (e.g. for embedding in New Chat) */
  className?: string
}

export function FilesPanel({ workspaceId, className }: FilesPanelProps) {
  const { token } = useAuth()
  const {
    data: tree,
    loading: treeLoading,
    error: treeError,
    refetch: fetchTree,
  } = useFetch(
    () => getFileTree(workspaceId, token!),
    [workspaceId, token],
    {
      enabled: !!token,
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load files"),
    }
  )

  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set(["."]))
  const [selectedFolderId, setSelectedFolderId] = useState(".")
  const [selectedFileId, setSelectedFileId] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [uploadMsg, setUploadMsg] = useState<{ text: string; isError: boolean } | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const folderInputRef = useRef<HTMLInputElement>(null)

  const {
    data: fileContent,
    loading: fileLoading,
    error: fileError,
  } = useFetch(
    () => getFileContent(workspaceId, selectedFileId!, token!),
    [workspaceId, selectedFileId, token],
    {
      enabled: !!(token && selectedFileId),
      errorMessage: (e) => (e instanceof Error ? e.message : "Failed to load file"),
    }
  )

  const handleToggle = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const handleFileSelect = useCallback((node: ExploreNode) => {
    if (node.type !== "file") return
    setSelectedFileId(node.id)
  }, [])

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
    <div className={className ?? "files-panel"}>
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
              {fileLoading && <p className="page-explore__viewer-loading">Loading…</p>}
              {fileError && (
                <p className="page-explore__viewer-error" role="alert">
                  Error: {fileError}
                </p>
              )}
              {!fileLoading && !fileError && (
                <pre className="page-explore__viewer-content">{fileContent ?? ""}</pre>
              )}
            </section>
          )}
        </div>
      </div>
    </div>
  )
}
