package db

// nextRevision returns the revision number that follows current.
//
// A row written before revisions existed reports 0 until the backfill migration
// gives it revision 1, and a store built in a test may never set the field at
// all. Both are treated as revision 1, so the first recorded edit lands on 2 and
// the numbers stay dense.
func nextRevision(current int) int {
	if current < 1 {
		return 2
	}
	return current + 1
}
