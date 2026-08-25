package objectstore

import (
	"path"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
)

// ArtifactObjectKey returns the object key holding one artifact's content.
//
// The shape mirrors the ownership question an operator asks of a bucket — whose
// data is this — and is disjoint from both the persist key space
// (<prefix>/<team_id>/home/) and the run-output key space
// (<prefix>/<user_id>/artifacts/), which are keyed differently and predate this.
func ArtifactObjectKey(prefix string, ref coreartifact.Ref) string {
	return path.Join(prefix, "teams", ref.TeamID, "artifacts", ref.ArtifactID, "content")
}
