package handlers

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// Page sizes. Three tiers rather than one number, because the routes differ in
// what a screen of results is worth: a browse list is read top to bottom, an
// operator's list is scanned and exported. Naming them makes a route's choice
// visible; before, eleven call sites each passed a pair of literals and no two
// readers could tell which differences were meant.
const (
	// browsePageDefault paginates lists a person reads a screen at a time --
	// revisions, workflow runs, an issue's flow.
	browsePageDefault, browsePageMax = 20, 100
	// listPageDefault paginates a team's own working lists.
	listPageDefault, listPageMax = 50, 100
	// bulkPageDefault paginates lists an operator scans or exports.
	bulkPageDefault, bulkPageMax = 50, 200
)

func parseLimitOffset(q url.Values, limitKey, offsetKey string, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if l := q.Get(limitKey); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxLimit {
			limit = n
		}
	}
	offset = 0
	if o := q.Get(offsetKey); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// pathValueInt reads a positive integer path segment, such as a revision
// number, and answers the request itself when it is missing or not one.
func pathValueInt(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	raw, ok := pathValueRequired(w, r, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		httputil.WriteJSONError(w, http.StatusBadRequest, key+" must be a positive integer")
		return 0, false
	}
	return n, true
}
