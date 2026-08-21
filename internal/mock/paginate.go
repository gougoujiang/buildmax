package mock

// paginate applies limit and offset to an already-filtered slice and reports
// the total before paging, matching what the database stores return.
//
// limit <= 0 means no limit here, because these doubles stand in for stores
// whose callers use 0 that way.
func paginate[T any](all []T, limit, offset int) ([]T, int) {
	total := len(all)
	if offset > total {
		offset = total
	}
	if offset < 0 {
		offset = 0
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, total
}
