import type { ApiArtifact } from "../../lib/api/types"
import { artifactLabel } from "./display"

/**
 * Ask before removing an artifact, in one place for every surface that offers
 * it.
 *
 * Deletion tombstones immediately at the authorization boundary, and content is
 * immutable — so re-uploading the same bytes produces a different reference,
 * and every link anyone saved to this one stays broken. That is worth stating
 * once, identically, rather than once per button.
 */
export function confirmArtifactDeletion(artifact: ApiArtifact): boolean {
  return window.confirm(
    `Delete ${artifactLabel(artifact)}?\n\n` +
      "Members lose access immediately, and anyone holding this reference will " +
      "find it gone. Re-uploading the file creates a different artifact, so the " +
      "reference cannot be restored."
  )
}
