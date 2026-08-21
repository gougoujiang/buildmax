package httputil

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Page sizes. Three tiers rather than one number, because the routes differ in
// what a screen of results is worth: a browse list is read top to bottom, an
// operator's list is scanned and exported. Naming them makes a route's choice
// visible; before, eleven call sites each passed a pair of literals and no two
// readers could tell which differences were meant.
const (
	// BrowsePageDefault paginates lists a person reads a screen at a time --
	// revisions, workflow runs, an issue's flow.
	BrowsePageDefault, BrowsePageMax = 20, 100
	// ListPageDefault paginates a team's own working lists.
	ListPageDefault, ListPageMax = 50, 100
	// BulkPageDefault paginates lists an operator scans or exports.
	BulkPageDefault, BulkPageMax = 50, 200
)

// RequireStore refuses when a feature's store is absent, which is the state of
// every feature on a deployment running without a database.
//
// store is `any` so one check serves every store interface. It compares against
// a nil interface, so a typed nil pointer would pass -- no caller passes one
// today, and the alternative is a check per store type.
func RequireStore(w http.ResponseWriter, store any, unavailable string) bool {
	if store == nil {
		WriteJSONError(w, http.StatusServiceUnavailable, unavailable)
		return false
	}
	return true
}

// PathValue reads a required path segment and answers the request itself when
// it is missing.
func PathValue(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	value := r.PathValue(key)
	if value == "" {
		WriteJSONError(w, http.StatusBadRequest, key+" required")
		return "", false
	}
	return value, true
}

// PathValueInt reads a positive integer path segment, such as a revision
// number.
func PathValueInt(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	raw, ok := PathValue(w, r, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		WriteJSONError(w, http.StatusBadRequest, key+" must be a positive integer")
		return 0, false
	}
	return n, true
}

// DecodeJSONBody reads a JSON request body, answering the request on malformed
// input.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// LimitOffset reads a page window, falling back to the default and clamping to
// the maximum. Values that are not numbers are ignored rather than refused: a
// bad limit is not worth failing a read over.
func LimitOffset(q url.Values, limitKey, offsetKey string, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if l := q.Get(limitKey); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxLimit {
			limit = n
		}
	}
	if o := q.Get(offsetKey); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
