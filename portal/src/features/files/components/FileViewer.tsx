interface FileViewerProps {
  selectedFileId: string | null
  selectedFileName: string | null
  fileLoading: boolean
  fileError: string | null
  fileContent: string | null
}

export function FileViewer({
  selectedFileId,
  selectedFileName,
  fileLoading,
  fileError,
  fileContent,
}: FileViewerProps) {
  if (!selectedFileId) return null

  return (
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
  )
}
