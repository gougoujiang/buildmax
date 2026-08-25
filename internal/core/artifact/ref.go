// Package artifact owns how an artifact's content object is addressed.
//
// The rest of the Artifact domain still lives in core/model and moves here
// later. What is here now is the one thing the service and the storage adapters
// both have to name, and neither may own: a service that took this type from
// the storage package would depend on an implementation, and a storage adapter
// that took it from the service would depend on a caller.
package artifact

// Ref names one artifact's content object.
//
// TeamID is here because it partitions the key space, not because callers
// address an artifact by team: an artifact is reached by its ar_ ID, and the
// service that holds the record supplies the team it belongs to.
type Ref struct {
	TeamID     string
	ArtifactID string
}
