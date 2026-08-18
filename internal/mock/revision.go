package mock

// pageRevisions applies a store's limit and offset to an already ordered slice.
// A limit of zero means no limit, matching the database store.
func pageRevisions[T any](all []T, limit, offset int) []T {
	if offset > 0 {
		if offset >= len(all) {
			return nil
		}
		all = all[offset:]
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all
}
