package localproject

import "context"

// Store is the persistence seam between the runtime and physical storage. It
// expresses Project semantics, not paths, mirroring session.Store: core owns
// what these operations mean, infra owns making them durable. See
// docs/design/local-project-memory.md §14.
type Store interface {
	// Resolve returns the Project registered for key, registering proposed
	// when there is none.
	//
	// Creation is serialized by the catalog writer lock and repeats the lookup
	// after acquiring it. That second lookup is the point: CLI and Desktop
	// starting in the same new repository at the same moment must agree on one
	// Project ID rather than mint two and split its memory. proposed is used
	// only on that path; when a Project already exists it is discarded and the
	// stored one's last_used_at is advanced.
	//
	// A locator claimed by two Projects is ErrDuplicateLocator: choosing one
	// would silently join or split a memory domain, so repair is the user's.
	Resolve(ctx context.Context, key Key, proposed Project) (Project, error)

	// Get returns one Project by ID, or ErrNotFound. It is the authoritative
	// read: it opens the bundle rather than trusting the catalog projection.
	Get(ctx context.Context, id string) (Project, error)

	// List returns the catalog projection, rebuilding it from the bundles when
	// it is missing or unusable.
	List(ctx context.Context) ([]Summary, error)

	// Update changes presentation or relinks a moved Project. It takes the
	// catalog writer lock, because a locator change has to be visible in the
	// projection at the same moment it is visible in the bundle.
	Update(ctx context.Context, id string, update Update) error

	// Delete removes a Project bundle and its row.
	//
	// It deletes nothing outside that bundle. Sessions are owned by the session
	// store and are not cascaded by a Project going away — a caller that means
	// to remove them says so there, having shown the user which ones. See
	// docs/design/local-project-memory.md §15.
	Delete(ctx context.Context, id string) error
}
