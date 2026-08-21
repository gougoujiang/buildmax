package team

// Fixtures this package's tests share. Copies of the root package's rather than
// an import: a helper crossing a package boundary makes the test boundary
// softer than the code's.

const (
	matrixSecret  = "matrix-secret"
	matrixTeam    = "tm_matrix"
	matrixOther   = "tm_other"
	matrixOwner   = "u_owner"
	matrixAdmin   = "u_admin"
	matrixMember  = "u_member"
	matrixOutside = "u_outsider"
)
