package db

// Page bounds for list queries that carry one. The ceiling is a safety cap: it
// stops a caller that asks for everything from reading an unbounded result set
// into memory, whatever the API above decided a page should be.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// clampPage applies the bounds above. limit <= 0 means the caller expressed no
// preference and gets the default, which is why this cannot be applied to a
// query whose callers use 0 to mean "all rows" -- see the note in
// docs/contribute/repo-layout.md on the queries that still lack a ceiling.
func clampPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
