package mock

import "time"

// seqEpoch anchors the deterministic, increasing instants used by in-memory
// stores whose persisted counterpart records time.Time values.
var seqEpoch = time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

func seqTime(n int) time.Time { return seqEpoch.Add(time.Duration(n) * time.Second) }

// paginate applies limit and offset to an already-filtered slice and reports
// the total before paging. A non-positive limit means no limit.
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

// pageRevisions applies a store's limit and offset to an already ordered slice.
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
