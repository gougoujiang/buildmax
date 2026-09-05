import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

/** The stored media type reduced to its lowercased type/subtype. */
export function mediaBase(mediaType: string): string {
  return mediaType.split(";")[0]?.trim().toLowerCase() ?? ""
}

interface ArtifactContentViewProps {
  filename: string
  mediaType: string
  /** Set for text-like content (text, markdown, html). */
  text?: string | null
  /** Set for binary content already turned into a blob URL (images). */
  objectUrl?: string | null
}

/**
 * ArtifactContentView renders already-fetched content the way its media type
 * calls for, and nothing more. It is shared by the authenticated detail preview
 * and the public share page so both render identically.
 *
 * Markdown goes through react-markdown with no raw-HTML plugin, so embedded
 * markup is escaped rather than run. HTML renders in an iframe sandboxed
 * WITHOUT allow-same-origin: a shared prototype's scripts run in an opaque
 * origin that cannot reach this page's session or the API as the viewer.
 */
export function ArtifactContentView({ filename, mediaType, text, objectUrl }: ArtifactContentViewProps) {
  if (objectUrl) {
    return <img className="artifact-preview__image" src={objectUrl} alt={filename} />
  }
  if (text == null) return null

  const base = mediaBase(mediaType)
  if (base === "text/html") {
    return (
      <iframe
        className="artifact-preview__frame"
        title={filename}
        sandbox="allow-scripts allow-popups allow-forms allow-modals"
        srcDoc={text}
      />
    )
  }
  if (base === "text/markdown") {
    return (
      <div className="artifact-preview__markdown">
        <Markdown remarkPlugins={[remarkGfm]}>{text}</Markdown>
      </div>
    )
  }
  return <pre className="artifact-preview__text">{text}</pre>
}
