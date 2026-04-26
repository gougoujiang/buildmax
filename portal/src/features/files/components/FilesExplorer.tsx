import { cn } from "../../../lib/cn"
import { useAuth } from "../../../contexts/AuthContext"
import { useTeam } from "../../../contexts/TeamContext"
import { FileList } from "./FileList"
import { FileTree } from "./FileTree"
import { FileViewer } from "./FileViewer"
import { useFilesExplorer } from "../hooks/useFilesExplorer"

interface FilesExplorerProps {
  className?: string
}

export function FilesExplorer({ className }: FilesExplorerProps) {
  const { token } = useAuth()
  const { currentTeamId } = useTeam()
  const explorer = useFilesExplorer({ teamId: currentTeamId, token })

  return (
    <div className={className ?? "files-panel"}>
      <div className="page-explore__upload-bar">
        <input
          ref={explorer.fileInputRef}
          type="file"
          multiple
          className="page-explore__file-input"
          onChange={explorer.handleUpload}
        />
        <input
          ref={explorer.folderInputRef}
          type="file"
          className="page-explore__file-input"
          onChange={explorer.handleFolderUpload}
          {...{ webkitdirectory: "", directory: "" }}
        />
        <button
          type="button"
          className="page-explore__upload-btn"
          disabled={explorer.uploading}
          onClick={() => explorer.fileInputRef.current?.click()}
        >
          {explorer.uploading ? "Uploading…" : "Upload Files"}
        </button>
        <button
          type="button"
          className="page-explore__upload-btn"
          disabled={explorer.uploading}
          onClick={() => explorer.folderInputRef.current?.click()}
        >
          {explorer.uploading ? "Uploading…" : "Upload Folder"}
        </button>
        {explorer.uploadMsg && (
          <span
            className={cn(
              "page-explore__upload-msg",
              explorer.uploadMsg.isError && "page-explore__upload-msg--error"
            )}
          >
            {explorer.uploadMsg.text}
          </span>
        )}
      </div>

      <div className="page-explore__panels">
        <FileTree
          tree={explorer.tree}
          treeLoading={explorer.treeLoading}
          treeError={explorer.treeError}
          expandedIds={explorer.expandedIds}
          selectedFolderId={explorer.selectedFolderId}
          onToggle={explorer.toggleFolder}
          onSelectFolder={explorer.selectFolder}
        />

        <div className="page-explore__content-panel">
          <FileList
            folderName={explorer.folderName}
            children={explorer.children}
            selectedFileId={explorer.selectedFileId}
            onSelectFolder={explorer.selectListFolder}
            onSelectFile={explorer.selectFile}
          />
          <FileViewer
            selectedFileId={explorer.selectedFileId}
            selectedFileName={explorer.selectedFileName}
            fileLoading={explorer.fileLoading}
            fileError={explorer.fileError}
            fileContent={explorer.fileContent}
          />
        </div>
      </div>
    </div>
  )
}
