import { apiFetch, getApiBase, parseErrorResponse, requestJson } from "../../lib/api/client"
import { authHeaders } from "../../lib/api/common"
import type {
  ApiArtifact,
  ApiArtifactList,
  ApiArtifactShare,
  ApiArtifactShareList,
  ApiSharedMeta,
} from "../../lib/api/types"

/**
 * The team route lists and receives; the id route reads one.
 *
 * An artifact's opaque id is its address, so everything after upload is reached
 * without naming a team — see docs/design/unified-artifacts.md section 6.1.
 */
export function artifactContentUrl(artifactId: string): string {
  return `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}/content`
}

/**
 * Read one artifact by id.
 *
 * A caller outside the owning team gets 404, exactly as a caller asking for an
 * id that never existed does — the server refuses to be an existence oracle, so
 * this reports "not found" for both rather than inventing a distinction.
 */
export async function getArtifact(artifactId: string, token: string): Promise<ApiArtifact> {
  const url = `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}`
  return requestJson<ApiArtifact>(url, { headers: authHeaders(token) })
}

export async function listArtifacts(
  teamId: string,
  token: string,
  options?: { limit?: number; offset?: number }
): Promise<ApiArtifactList> {
  const params = new URLSearchParams()
  if (options?.limit != null) params.set("limit", String(options.limit))
  if (options?.offset != null) params.set("offset", String(options.offset))
  const query = params.toString()
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/artifacts${query ? `?${query}` : ""}`
  return requestJson<ApiArtifactList>(url, { headers: authHeaders(token) })
}

export async function uploadArtifact(
  teamId: string,
  token: string,
  file: File,
  title?: string
): Promise<ApiArtifact> {
  const form = new FormData()
  form.append("file", file, file.name)
  // The title travels in the query so it does not depend on field order: the
  // server streams the file part straight to storage and never reads past it.
  const query = title && title.trim() !== "" ? `?title=${encodeURIComponent(title.trim())}` : ""
  const url = `${getApiBase()}/api/teams/${encodeURIComponent(teamId)}/artifacts${query}`
  const res = await apiFetch(url, { method: "POST", headers: authHeaders(token), body: form })
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Upload failed"))
  }
  return (await res.json()) as ApiArtifact
}

export async function deleteArtifact(artifactId: string, token: string): Promise<void> {
  const url = `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}`
  const res = await apiFetch(url, { method: "DELETE", headers: authHeaders(token) })
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Delete failed"))
  }
}

// --- Share management (authenticated) ---

export async function createShare(artifactId: string, token: string): Promise<ApiArtifactShare> {
  const url = `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}/shares`
  const res = await apiFetch(url, { method: "POST", headers: authHeaders(token) })
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Could not create a public link"))
  }
  return (await res.json()) as ApiArtifactShare
}

export async function listShares(artifactId: string, token: string): Promise<ApiArtifactShareList> {
  const url = `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}/shares`
  return requestJson<ApiArtifactShareList>(url, { headers: authHeaders(token) })
}

export async function revokeShare(artifactId: string, shareId: string, token: string): Promise<void> {
  const url = `${getApiBase()}/api/artifacts/${encodeURIComponent(artifactId)}/shares/${encodeURIComponent(shareId)}`
  const res = await apiFetch(url, { method: "DELETE", headers: authHeaders(token) })
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Could not revoke the link"))
  }
}

// --- Public share access (no session) ---

/** sharedRawUrl is the token-authorized content URL an iframe or link uses. */
export function sharedRawUrl(shareToken: string, download = false): string {
  const base = `${getApiBase()}/shared/artifacts/${encodeURIComponent(shareToken)}/raw`
  return download ? `${base}?dl=1` : base
}

export async function fetchSharedMeta(shareToken: string): Promise<ApiSharedMeta> {
  const res = await fetch(`${getApiBase()}/shared/artifacts/${encodeURIComponent(shareToken)}/meta`)
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "This link is not available"))
  }
  return (await res.json()) as ApiSharedMeta
}

/** Fetch shared content with no session, as text or a blob URL. */
export async function fetchSharedContent(
  shareToken: string
): Promise<{ text?: string; objectUrl?: string; mediaType: string }> {
  const res = await fetch(sharedRawUrl(shareToken))
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "This link is not available"))
  }
  const mediaType = res.headers.get("Content-Type") ?? ""
  if (mediaType.startsWith("text/")) {
    return { text: await res.text(), mediaType }
  }
  return { objectUrl: URL.createObjectURL(await res.blob()), mediaType }
}

/**
 * Fetch content the server said is safe to display, as a blob URL.
 *
 * The decision is the server's `inline` flag, not a guess from the media type:
 * anything that could carry script is served as a download and must not be put
 * in front of a viewer here.
 */
export async function fetchArtifactPreview(
  artifactId: string,
  token: string
): Promise<{ text?: string; objectUrl?: string; mediaType: string }> {
  const res = await apiFetch(artifactContentUrl(artifactId), { headers: authHeaders(token) })
  if (!res.ok) {
    throw new Error(await parseErrorResponse(res, "Preview failed"))
  }
  const mediaType = res.headers.get("Content-Type") ?? ""
  if (mediaType.startsWith("text/")) {
    return { text: await res.text(), mediaType }
  }
  return { objectUrl: URL.createObjectURL(await res.blob()), mediaType }
}
