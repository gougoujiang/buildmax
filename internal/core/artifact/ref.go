package artifact

// Ref names one artifact's content object.
//
// It lives in core rather than beside either user because the service and the
// storage adapters both have to name it and neither may own it: a service that
// took this type from the storage package would depend on an implementation,
// and a storage adapter that took it from the service would depend on a caller.
//
// TeamID is here because it partitions the key space, not because callers
// address an artifact by team: an artifact is reached by its opaque ID, and the
// service that holds the record supplies the team it belongs to.
type Ref struct {
	TeamID     string
	ArtifactID string
}
