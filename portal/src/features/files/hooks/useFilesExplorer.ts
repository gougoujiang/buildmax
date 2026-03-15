import { useCallback, useRef, useState } from "react"
import type { ExploreNode } from "../../../lib/types"
import { findNodeById, getChildren } from "../../../lib/explore"
import { getErrorMessage } from "../../../lib/errorMessage"
import { useFetch } from "../../../hooks/useFetch"
import { getFileContent, getFileTree, uploadFiles } from "../api"

interface UseFilesExplorerOptions {
  profileId: string
  token: string | null
}

export function useFilesExplorer({ profileId, token }: UseFilesExplorerOptions) {
  const {
    data: tree,
    loading: treeLoading,
    error: treeError,
    refetch: refetchTree,
  } = useFetch(
    () => getFileTree(profileId, token!),
    [profileId, token],
    {
      enabled: !!token,
      errorMessage: (e) => getErrorMessage(e, "Failed to load files"),
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
    () => getFileContent(profileId, selectedFileId!, token!),
    [profileId, selectedFileId, token],
    {
      enabled: !!(token && selectedFileId),
      errorMessage: (e) => getErrorMessage(e, "Failed to load file"),
    }
  )

  const toggleFolder = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const selectFolder = useCallback((folderId: string) => {
    setSelectedFolderId(folderId)
  }, [])

  const selectListFolder = useCallback((folderId: string) => {
    setSelectedFolderId(folderId)
    setExpandedIds((prev) => new Set(prev).add(folderId))
    setSelectedFileId(null)
  }, [])

  const selectFile = useCallback((node: ExploreNode) => {
    if (node.type !== "file") return
    setSelectedFileId(node.id)
  }, [])

  const doUpload = useCallback(
    async (files: File[], paths?: string[], options?: { maxFiles?: number }) => {
      if (!token) {
        setUploadMsg({ text: "Not authenticated", isError: true })
        return
      }
      const { maxFiles } = options ?? {}
      if (maxFiles != null && files.length > maxFiles) {
        setUploadMsg({ text: `Too many files (max ${maxFiles})`, isError: true })
        return
      }
      setUploading(true)
      setUploadMsg(null)
      try {
        const res = await uploadFiles(profileId, files, token, paths)
        setUploadMsg({ text: `Uploaded ${res.uploaded.length} file(s)`, isError: false })
        await refetchTree()
      } catch (err) {
        setUploadMsg({ text: getErrorMessage(err, "Upload failed"), isError: true })
      } finally {
        setUploading(false)
      }
    },
    [profileId, token, refetchTree]
  )

  const handleUpload = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files
      if (!selected || selected.length === 0) return
      const files = Array.from(selected)
      try {
        await doUpload(files, undefined, { maxFiles: 10 })
      } finally {
        if (fileInputRef.current) fileInputRef.current.value = ""
      }
    },
    [doUpload]
  )

  const handleFolderUpload = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const selected = e.target.files
      if (!selected || selected.length === 0) return
      const files = Array.from(selected)
      const paths = files.map((f) => f.webkitRelativePath)
      try {
        await doUpload(files, paths)
      } finally {
        if (folderInputRef.current) folderInputRef.current.value = ""
      }
    },
    [doUpload]
  )

  const children = tree ? getChildren(tree, selectedFolderId) : []
  const selectedFolderNode = tree ? findNodeById(tree, selectedFolderId) : undefined
  const folderName = selectedFolderId === "." ? "home" : selectedFolderNode?.name ?? "—"
  const selectedFileName =
    tree && selectedFileId ? findNodeById(tree, selectedFileId)?.name ?? null : null

  return {
    tree,
    treeLoading,
    treeError,
    expandedIds,
    selectedFolderId,
    selectedFileId,
    fileContent,
    fileLoading,
    fileError,
    fileInputRef,
    folderInputRef,
    uploading,
    uploadMsg,
    children,
    folderName,
    selectedFileName,
    toggleFolder,
    selectFolder,
    selectListFolder,
    selectFile,
    handleUpload,
    handleFolderUpload,
  }
}
